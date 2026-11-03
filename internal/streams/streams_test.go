package streams

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

func TestRenderParseRoundTrip(t *testing.T) {
	for _, p := range []Proxy{
		{Name: "postgres", Port: 5432, Upstream: "10.0.0.20:5432", ConnectTimeout: "10s", Timeout: "10m", IPListName: "office"},
		{Name: "dns", Port: 53, UDP: true, Upstream: "[2001:db8::53]:53"},
	} {
		out, err := Render(p, "/etc/nginx/snippets")
		if err != nil {
			t.Fatal(err)
		}
		if got := Parse(p.Name, out); !reflect.DeepEqual(got, p) {
			t.Errorf("round trip = %+v, want %+v\n%s", got, p, out)
		}
	}
	out, _ := Render(Proxy{Name: "dns", Port: 53, UDP: true, Upstream: "10.0.0.53:53"}, "/s")
	if !strings.Contains(out, "listen 53 udp;") {
		t.Errorf("missing udp listener:\n%s", out)
	}
}

func TestValidate(t *testing.T) {
	ok := Proxy{Name: "pg", Port: 5432, Upstream: "10.0.0.1:5432"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Proxy){
		"name":     func(p *Proxy) { p.Name = "a/b" },
		"port":     func(p *Proxy) { p.Port = 70000 },
		"upstream": func(p *Proxy) { p.Upstream = "10.0.0.1" },
		"inject":   func(p *Proxy) { p.Upstream = "10.0.0.1:5432;deny" },
		"timeout":  func(p *Proxy) { p.Timeout = "10 m" },
	} {
		p := ok
		mutate(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func newService(t *testing.T, nginxConf string) *Service {
	t.Helper()
	root := t.TempDir()
	s := &Service{
		Dir: filepath.Join(root, "gonix-streams"), MainIncludeDir: filepath.Join(root, "modules-enabled"),
		ConfigDir: root, SnippetsDir: filepath.Join(root, "snippets"), Writer: &snippet.Writer{Tester: okTester{}},
	}
	os.MkdirAll(s.MainIncludeDir, 0o755)
	if err := os.WriteFile(filepath.Join(root, "nginx.conf"), []byte(strings.ReplaceAll(nginxConf, "ROOT", root)), 0o644); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSetUp(t *testing.T) {
	// Debian layout: modules-enabled is included at top level.
	s := newService(t, "include ROOT/modules-enabled/*.conf;\nevents {}\nhttp {}\n")
	if ready, _ := s.Ready(); ready {
		t.Fatal("should not be ready before setup")
	}
	if !s.CanSetUp() {
		t.Fatal("expected automatic setup to be possible")
	}
	if err := s.SetUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ready, _ := s.Ready(); !ready {
		t.Error("should be ready after setup")
	}
	data, _ := os.ReadFile(s.WrapperPath())
	if !strings.Contains(string(data), "include "+s.Dir+"/*.conf;") {
		t.Errorf("wrapper = %s", data)
	}

	// An existing stream block must not get a second one.
	s = newService(t, "include ROOT/modules-enabled/*.conf;\nstream {\n}\nhttp {}\n")
	if s.CanSetUp() {
		t.Error("must not set up a second stream block")
	}
	if _, why := s.Ready(); !strings.Contains(why, "include "+s.Dir+"/*.conf;") {
		t.Errorf("instructions = %q", why)
	}

	// The admin may include the directory in their own stream block.
	s = newService(t, "stream {\n    include ROOT/gonix-streams/*.conf;\n}\n")
	if ready, _ := s.Ready(); !ready {
		t.Error("an existing include of the streams directory should count as ready")
	}
}

func TestLifecycle(t *testing.T) {
	s := newService(t, "events {}\n")
	ctx := context.Background()
	p := Proxy{Name: "pg", Port: 5432, Upstream: "10.0.0.20:5432", Enabled: true}
	if err := s.Save(ctx, p, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, Proxy{Name: "pg2", Port: 5432, Upstream: "10.0.0.21:5432", Enabled: true}, true); err == nil {
		t.Error("expected a port conflict")
	}
	if err := s.Save(ctx, Proxy{Name: "dns", Port: 5432, UDP: true, Upstream: "10.0.0.53:53", Enabled: true}, true); err != nil {
		t.Errorf("same port over UDP should be allowed: %v", err)
	}
	if err := s.SetEnabled(ctx, "pg", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "pg.conf")); !os.IsNotExist(err) {
		t.Error("disabled proxy must not leave an active .conf file")
	}
	list, _ := s.List()
	if len(list) != 2 || list[1].Name != "pg" || list[1].Enabled {
		t.Errorf("List = %+v", list)
	}
	if err := s.Delete(ctx, "pg"); err != nil {
		t.Fatal(err)
	}
	if s.Exists("pg") {
		t.Error("pg should be gone")
	}
}

func TestUsingIPList(t *testing.T) {
	s := newService(t, "events {}\n")
	if err := s.Save(context.Background(), Proxy{Name: "pg", Port: 5432, Upstream: "10.0.0.1:5432", IPListName: "office", Enabled: true}, true); err != nil {
		t.Fatal(err)
	}
	if got := s.UsingIPList("office"); !reflect.DeepEqual(got, []string{"stream pg"}) {
		t.Errorf("UsingIPList = %v", got)
	}
}
