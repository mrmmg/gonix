package transfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

type server struct {
	dirs Dirs
	mgr  *nginx.Manager
}

func newServer(t *testing.T) server {
	t.Helper()
	root := t.TempDir()
	d := Dirs{
		SitesAvailable: filepath.Join(root, "sites-available"), SitesEnabled: filepath.Join(root, "sites-enabled"),
		Snippets: filepath.Join(root, "snippets"), HTTP: filepath.Join(root, "conf.d"),
		AccessLists: filepath.Join(root, "accesslists"), Streams: filepath.Join(root, "streams"), Main: filepath.Join(root, "modules"),
	}
	for _, dir := range []string{d.SitesAvailable, d.SitesEnabled, d.Snippets, d.HTTP, d.AccessLists, d.Streams, d.Main} {
		os.MkdirAll(dir, 0o755)
	}
	r, err := nginx.NewRenderer(root, root, d.AccessLists, d.Snippets)
	if err != nil {
		t.Fatal(err)
	}
	return server{dirs: d, mgr: nginx.NewManager(d.SitesAvailable, d.SitesEnabled, r)}
}

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func TestExportImportBetweenLayouts(t *testing.T) {
	src := newServer(t)
	h := nginx.Host{ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80, IPListName: "office", AccessListName: "admins",
		Locations: []nginx.Location{{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}}}}
	if err := src.mgr.WriteHost(h); err != nil {
		t.Fatal(err)
	}
	src.mgr.Enable("example.com")
	if err := src.mgr.WriteHost(nginx.Host{ServerName: "off.example.com", Mode: nginx.ModeStatic, Root: "/srv", Listen: 80}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src.dirs.SitesAvailable, "handwritten"), []byte("server {}\n"), 0o644)
	os.WriteFile(filepath.Join(src.dirs.Snippets, "gonix-ip-office.conf"), []byte("allow 10.0.0.0/8;\ndeny all;\n"), 0o644)
	os.WriteFile(filepath.Join(src.dirs.Snippets, "someone-elses.conf"), []byte("x\n"), 0o644)
	os.WriteFile(filepath.Join(src.dirs.HTTP, "gonix-realip.conf"), []byte("real_ip_header X-Real-IP;\n"), 0o644)
	os.WriteFile(filepath.Join(src.dirs.AccessLists, "admins.htpasswd"), []byte("admin:$2y$hash\n"), 0o640)
	os.WriteFile(filepath.Join(src.dirs.Streams, "pg.conf.disabled"), []byte("server {}\n"), 0o644)

	var buf bytes.Buffer
	m, err := Export(&buf, src.dirs, src.mgr)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Enabled) != 1 || m.Enabled[0] != "example.com" {
		t.Errorf("manifest enabled = %v", m.Enabled)
	}

	dst := newServer(t) // different directories
	plan, err := Read(bytes.NewReader(buf.Bytes()), dst.dirs)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range plan.Files {
		names = append(names, f.Section+"/"+f.Name)
	}
	got := strings.Join(names, " ")
	for _, want := range []string{"sites/example.com", "sites/off.example.com", "snippets/gonix-ip-office.conf", "http/gonix-realip.conf", "accesslists/admins.htpasswd", "streams/pg.conf.disabled"} {
		if !strings.Contains(got, want) {
			t.Errorf("plan is missing %s: %s", want, got)
		}
	}
	for _, unwanted := range []string{"handwritten", "someone-elses"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("plan must not include %s: %s", unwanted, got)
		}
	}

	im := &Importer{Dirs: dst.dirs, Manager: dst.mgr, Writer: &snippet.Writer{Tester: okTester{}}}
	if err := im.Apply(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	site, _ := os.ReadFile(filepath.Join(dst.dirs.SitesAvailable, "example.com"))
	if strings.Contains(string(site), src.dirs.Snippets) || !strings.Contains(string(site), filepath.Join(dst.dirs.Snippets, "gonix-ip-office.conf")) ||
		!strings.Contains(string(site), filepath.Join(dst.dirs.AccessLists, "admins.htpasswd")) {
		t.Errorf("paths not rewritten:\n%s", site)
	}
	if !dst.mgr.IsEnabled("example.com") || dst.mgr.IsEnabled("off.example.com") {
		t.Error("enabled state not reproduced")
	}
	if info, _ := os.Stat(filepath.Join(dst.dirs.AccessLists, "admins.htpasswd")); info.Mode().Perm() != 0o640 {
		t.Errorf("htpasswd mode = %v", info.Mode().Perm())
	}

	// Importing again is a no-op; a changed local file is a conflict.
	plan, _ = Read(bytes.NewReader(buf.Bytes()), dst.dirs)
	if c := plan.Conflicts(); len(c) != 0 {
		t.Errorf("unexpected conflicts: %v", c)
	}
	os.WriteFile(filepath.Join(dst.dirs.HTTP, "gonix-realip.conf"), []byte("changed\n"), 0o644)
	plan, _ = Read(bytes.NewReader(buf.Bytes()), dst.dirs)
	if err := im.Apply(context.Background(), plan, false); err == nil {
		t.Error("expected a conflict error")
	}
	if err := im.Apply(context.Background(), plan, true); err != nil {
		t.Fatal(err)
	}
}

func TestReadRejectsUnsafeEntries(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	writeEntry(tw, manifestName, []byte(`{"version":1}`), 0o644)
	writeEntry(tw, "sites/../../etc/passwd", []byte("x"), 0o644)
	writeEntry(tw, "snippets/../evil.conf", []byte("x"), 0o644)
	writeEntry(tw, "unknown/x", []byte("x"), 0o644)
	writeEntry(tw, "sites/ok.example.com", []byte("server {}\n"), 0o644)
	tw.Close()
	gz.Close()

	plan, err := Read(&buf, newServer(t).dirs)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Name != "ok.example.com" {
		t.Errorf("files = %+v", plan.Files)
	}
	if _, err := Read(strings.NewReader("not gzip"), Dirs{}); err == nil {
		t.Error("expected an error for a non-archive")
	}
}
