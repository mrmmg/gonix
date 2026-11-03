package npm

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/streams"
)

func scanFixtures(t *testing.T) map[string]Item {
	t.Helper()
	items, err := Scan("testdata/data", "testdata/letsencrypt")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Item{}
	for _, it := range items {
		byName[it.Name()] = it
	}
	return byName
}

func TestScanProxyHost(t *testing.T) {
	it := scanFixtures(t)["app.example.com"]
	h := it.Host
	if h == nil || it.Skip != "" {
		t.Fatalf("app.example.com = %+v", it)
	}
	if !reflect.DeepEqual(h.ServerAlias, []string{"www.app.example.com"}) || h.Mode != nginx.ModeReverseProxy {
		t.Errorf("host = %+v", h)
	}
	p := h.Locations[0].Proxy
	if p.UpstreamHost != "192.168.1.10" || p.UpstreamPort != 8080 || p.UpstreamScheme != "http" || !p.WebSocket {
		t.Errorf("proxy = %+v", p)
	}
	if len(h.Locations) != 2 || h.Locations[1].Path != "/api" || h.Locations[1].Proxy.UpstreamPort != 9000 {
		t.Errorf("locations = %+v", h.Locations)
	}
	if !h.SSL.Enabled || !h.SSL.HTTP2 || !h.SSL.ForceHTTPS || it.CertFrom != filepath.Join("testdata/letsencrypt", "live/npm-1") {
		t.Errorf("ssl = %+v from %q", h.SSL, it.CertFrom)
	}
	for _, want := range []string{"block-sensitive-files", "block-bad-bots", "hsts", "static-cache"} {
		found := false
		for _, p := range h.Presets {
			found = found || p == want
		}
		if !found {
			t.Errorf("preset %s missing: %v", want, h.Presets)
		}
	}
	if it.AccessName != "npm-2" || it.IPListName != "npm-2" || !reflect.DeepEqual(it.IPAllow, []string{"10.0.0.0/8"}) || !h.SatisfyAny {
		t.Errorf("access = %q %q %v satisfyAny=%v", it.AccessName, it.IPListName, it.IPAllow, h.SatisfyAny)
	}
	warnings := strings.Join(it.Warnings, "\n")
	if !strings.Contains(warnings, "client_max_body_size 100m;") || !strings.Contains(warnings, "/files not imported") {
		t.Errorf("warnings = %s", warnings)
	}
}

func TestScanOtherKinds(t *testing.T) {
	items := scanFixtures(t)
	if _, ok := items["disabled.example.com"]; ok {
		t.Error("a host disabled in NPM has no server block and must be skipped")
	}
	plain := items["plain.example.com"]
	if p := plain.Host.Locations[0].Proxy; p.UpstreamScheme != "https" || p.UpstreamHost != "backend.internal" || p.UpstreamPort != 8443 || p.WebSocket {
		t.Errorf("plain proxy = %+v", p)
	}
	if plain.Host.SSL.Enabled || len(plain.Warnings) != 0 {
		t.Errorf("plain = %+v warnings %v", plain.Host.SSL, plain.Warnings)
	}
	r := items["old.example.com"].Host
	if r.Mode != nginx.ModeRedirect || r.Redirect != (nginx.RedirectConfig{Target: "https://new.example.com", Code: 302, PreservePath: true}) {
		t.Errorf("redirect = %+v", r.Redirect)
	}
	tcp, udp := items["stream npm-5432-tcp"], items["stream npm-5432-udp"]
	if tcp.Stream == nil || udp.Stream == nil || !udp.Stream.UDP || tcp.Stream.Upstream != "10.0.0.20:5432" {
		t.Errorf("streams = %+v / %+v", tcp.Stream, udp.Stream)
	}
	var wild Item
	for _, it := range items {
		if strings.HasSuffix(it.Source, "3.conf") && strings.Contains(it.Source, "proxy_host") {
			wild = it
		}
	}
	if wild.Skip == "" {
		t.Error("wildcard-only host should be skipped")
	}
	if _, err := Scan(t.TempDir(), ""); err == nil {
		t.Error("expected an error for a directory that is not NPM's")
	}
}

func TestRun(t *testing.T) {
	items, err := Scan("testdata/data", "testdata/letsencrypt")
	if err != nil {
		t.Fatal(err)
	}
	certs, access := t.TempDir(), t.TempDir()
	Prepare(items, Target{CertsDir: certs, Exists: func(n string) bool { return n == "plain.example.com" }})

	var created []nginx.Host
	var lists, savedStreams []string
	r := &Runner{
		CopyFile: CopyFile, CertsDir: certs, AccessDir: access,
		CreateIPList: func(_ context.Context, name string, allow bool, entries []string) error {
			lists = append(lists, name)
			return nil
		},
		EnsurePreset: func(context.Context, []string) error { return nil },
		CreateHost:   func(_ context.Context, h nginx.Host) error { created = append(created, h); return nil },
		SaveStream: func(_ context.Context, p streams.Proxy) error {
			savedStreams = append(savedStreams, p.Name)
			return nil
		},
	}
	res := r.Run(context.Background(), items)

	if len(created) != 2 || len(savedStreams) != 2 || !reflect.DeepEqual(lists, []string{"npm-2"}) {
		t.Fatalf("created=%d streams=%v lists=%v\nresult=%+v", len(created), savedStreams, lists, res)
	}
	app := created[0]
	if app.ServerName != "app.example.com" || app.SSL.CertFile != filepath.Join(certs, "app.example.com", "fullchain.pem") ||
		app.AccessListName != "npm-2" || app.IPListName != "npm-2" {
		t.Errorf("app = %+v", app)
	}
	if err := app.Validate(); err != nil {
		t.Errorf("imported host does not validate: %v", err)
	}
	if info, err := os.Stat(filepath.Join(certs, "app.example.com", "privkey.pem")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("private key copy: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(access, "npm-2.htpasswd")); err != nil {
		t.Error("access list not copied")
	}
	skipped := strings.Join(res.Skipped, "\n")
	if !strings.Contains(skipped, "plain.example.com: a host with this name already exists") || !strings.Contains(skipped, "wildcards") {
		t.Errorf("skipped = %s", skipped)
	}
}
