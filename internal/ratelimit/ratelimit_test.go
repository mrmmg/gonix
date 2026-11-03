package ratelimit

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/nginxtest"
	"github.com/mrmmg/gonix/internal/snippet"
)

func TestRenderParseRoundTrip(t *testing.T) {
	cases := append([]Limit{{Name: "plain", Rate: 1, Per: "s"}}, Presets...)
	for _, l := range cases {
		zone, err := RenderZone(l)
		if err != nil {
			t.Fatal(err)
		}
		apply, err := RenderApply(l)
		if err != nil {
			t.Fatal(err)
		}
		if got := Parse(l.Name, zone, apply); !reflect.DeepEqual(got, l) {
			t.Errorf("round trip = %+v, want %+v\n%s\n%s", got, l, zone, apply)
		}
	}

	apply, _ := RenderApply(Limit{Name: "api", Rate: 10, Per: "s", Burst: 20, NoDelay: true})
	if !strings.Contains(apply, "limit_req zone=gonix_api burst=20 nodelay;") || !strings.Contains(apply, "limit_req_status 429;") {
		t.Errorf("unexpected apply snippet:\n%s", apply)
	}
}

func TestValidate(t *testing.T) {
	for _, l := range []Limit{
		{Name: "a b", Rate: 1, Per: "s"},
		{Name: "a", Rate: 0, Per: "s"},
		{Name: "a", Rate: 1, Per: "h"},
		{Name: "a", Rate: 1, Per: "s", Burst: -1},
		{Name: "a", Rate: 1, Per: "s", Conn: -1},
	} {
		if err := l.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want error", l)
		}
	}
}

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func TestServiceLifecycle(t *testing.T) {
	snippets := t.TempDir()
	r, err := nginx.NewRenderer(t.TempDir(), t.TempDir(), t.TempDir(), snippets)
	if err != nil {
		t.Fatal(err)
	}
	mgr := nginx.NewManager(t.TempDir(), t.TempDir(), r)
	svc := &Service{SnippetsDir: snippets, HTTPDir: filepath.Join(t.TempDir(), "conf.d"), Manager: mgr, Writer: &snippet.Writer{Tester: okTester{}}}
	ctx := context.Background()

	l := Limit{Name: "api", Rate: 10, Per: "s", Burst: 20, NoDelay: true}
	if err := svc.Create(ctx, l); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Create(ctx, l); err == nil {
		t.Error("expected duplicate Create to fail")
	}
	l.Conn = 10
	if err := svc.Update(ctx, l); err != nil {
		t.Fatalf("Update: %v", err)
	}
	all, err := svc.List()
	if err != nil || len(all) != 1 || !reflect.DeepEqual(all[0], l) {
		t.Fatalf("List = %+v, %v", all, err)
	}

	h := nginx.Host{ServerName: "example.com", Mode: nginx.ModeStatic, Root: "/srv", Listen: 80, RateLimit: "api"}
	if err := mgr.WriteHost(h); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "api"); err == nil {
		t.Error("expected Delete to refuse a limit in use")
	}
	if err := os.Remove(filepath.Join(mgr.SitesAvailable, "example.com")); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "api"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for _, p := range []string{svc.ZonePath("api"), svc.ApplyPath("api")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be removed", p)
		}
	}
}

func TestHostWithRateLimitsPassesRealNginx(t *testing.T) {
	p := nginxtest.New(t)
	snippets := filepath.Join(p.Dir, "snippets")
	if err := os.MkdirAll(snippets, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{SnippetsDir: snippets, HTTPDir: p.HTTPDir, Writer: &snippet.Writer{Tester: okTester{}}}
	for _, l := range Presets {
		if err := svc.Create(context.Background(), l); err != nil {
			t.Fatal(err)
		}
	}

	r, err := nginx.NewRenderer(p.Dir, p.Dir, p.Dir, snippets)
	if err != nil {
		t.Fatal(err)
	}
	h := nginx.Host{
		ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80, RateLimit: "general",
		Locations: []nginx.Location{
			{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}},
			{Path: "/login", RateLimit: "login", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}},
		},
	}
	out, err := r.Render(h)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := nginx.ParseManagedHost(out)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.RateLimit != "general" || parsed.Locations[0].RateLimit != "" || parsed.Locations[1].RateLimit != "login" {
		t.Errorf("round trip lost rate limits: host=%q locations=%q,%q",
			parsed.RateLimit, parsed.Locations[0].RateLimit, parsed.Locations[1].RateLimit)
	}
	p.AddSite(t, "example.com", out)
	p.Test(t)
}
