package presets

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/nginxtest"
	"github.com/mrmmg/gonix/internal/snippet"
)

func TestPresetNamesAreValidSnippetNames(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range All {
		if !nginx.ValidSnippetName(p.Name) || seen[p.Name] {
			t.Errorf("preset name %q is invalid or duplicated", p.Name)
		}
		seen[p.Name] = true
	}
}

func TestApplicableAndSort(t *testing.T) {
	plain := nginx.Host{Mode: nginx.ModeReverseProxy}
	for _, p := range Applicable(plain) {
		if p.SSLOnly {
			t.Errorf("%s offered to a host without SSL", p.Name)
		}
	}
	for _, p := range Applicable(nginx.Host{Mode: nginx.ModeStatic}) {
		if p.ProxyOnly {
			t.Errorf("%s offered to a static host", p.Name)
		}
	}
	redirect := nginx.Host{Mode: nginx.ModeRedirect, SSL: nginx.SSLConfig{Enabled: true}}
	got := Applicable(redirect)
	if len(got) != 1 || got[0].Name != "hsts" {
		t.Errorf("redirect host presets = %v, want only hsts", got)
	}
	if s := Sort([]string{"gzip", "nope", "security-headers", "gzip"}); !reflect.DeepEqual(s, []string{"security-headers", "gzip"}) {
		t.Errorf("Sort = %v", s)
	}
}

type countingTester struct{ n int }

func (c *countingTester) Test(context.Context) (nginx.Result, error) {
	c.n++
	return nginx.Result{OK: true}, nil
}

func TestEnsureWritesOnlyChangedFiles(t *testing.T) {
	tester := &countingTester{}
	svc := &Service{SnippetsDir: t.TempDir(), HTTPDir: filepath.Join(t.TempDir(), "conf.d"), Writer: &snippet.Writer{Tester: tester}}
	ctx := context.Background()
	if err := svc.Ensure(ctx, []string{"gzip", "static-cache"}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	for _, path := range []string{svc.ServerPath("gzip"), svc.ServerPath("static-cache"), svc.HTTPPath("static-cache")} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s: %v", path, err)
		}
	}
	if err := svc.Ensure(ctx, []string{"gzip", "static-cache"}); err != nil {
		t.Fatalf("Ensure again: %v", err)
	}
	if tester.n != 1 {
		t.Errorf("nginx -t ran %d times, want 1 (unchanged files are not rewritten)", tester.n)
	}
	if err := svc.Ensure(ctx, []string{"nope"}); err == nil {
		t.Error("expected an error for an unknown preset")
	}
}

// Every preset, together with HTTP/2 and HTTP/3, must be accepted by a real
// Nginx, both on a proxy host and (for those that apply) a redirect host.
func TestAllPresetsPassRealNginx(t *testing.T) {
	p := nginxtest.New(t)
	snippets := filepath.Join(p.Dir, "snippets")
	svc := &Service{SnippetsDir: snippets, HTTPDir: p.HTTPDir, CacheDir: filepath.Join(p.Dir, "cache"), Writer: &snippet.Writer{Tester: &countingTester{}}}
	if err := os.MkdirAll(snippets, 0o755); err != nil {
		t.Fatal(err)
	}
	var all []string
	for _, pr := range All {
		all = append(all, pr.Name)
	}
	if err := svc.Ensure(context.Background(), all); err != nil {
		t.Fatal(err)
	}

	build, err := nginx.DetectBuild(context.Background(), "nginx")
	if err != nil {
		t.Fatal(err)
	}
	r, err := nginx.NewRenderer(p.Dir, p.Dir, p.Dir, snippets)
	if err != nil {
		t.Fatal(err)
	}
	r.Build = build

	cert, key := p.SelfSignedCert(t, "example.com")
	ssl := nginx.SSLConfig{Enabled: true, CertFile: cert, KeyFile: key, HTTP2: true, HTTP3: build.SupportsHTTP3(), ForceHTTPS: true}
	proxy := nginx.Host{
		ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80, ListenSSL: 443, SSL: ssl,
		Presets: all,
		Locations: []nginx.Location{{Path: "/", Proxy: &nginx.ProxyConfig{
			UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}}},
	}
	redirect := nginx.Host{
		ServerName: "example.net", Mode: nginx.ModeRedirect, Listen: 80, ListenSSL: 443, SSL: ssl,
		Presets:  []string{"hsts"},
		Redirect: nginx.RedirectConfig{Target: "https://example.com", Code: 301, PreservePath: true},
	}
	for _, h := range []nginx.Host{proxy, redirect} {
		out, err := r.Render(h)
		if err != nil {
			t.Fatal(err)
		}
		if build.HTTP2Directive() && (!strings.Contains(out, "http2 on;") || strings.Contains(out, "ssl http2")) {
			t.Errorf("expected the http2 directive on nginx %d.%d:\n%s", build.Major, build.Minor, out)
		}
		parsed, err := nginx.ParseManagedHost(out)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(parsed.Presets, h.Presets) || parsed.SSL.HTTP2 != true || parsed.SSL.HTTP3 != h.SSL.HTTP3 || parsed.Listen != 80 {
			t.Errorf("round trip lost settings: presets=%v http2=%v http3=%v listen=%d",
				parsed.Presets, parsed.SSL.HTTP2, parsed.SSL.HTTP3, parsed.Listen)
		}
		p.AddSite(t, h.ServerName, out)
	}
	out, ok := p.Run(t)
	if !ok {
		t.Fatalf("nginx -t rejected the configuration:\n%s", out)
	}
	if strings.Contains(out, "[warn]") {
		t.Errorf("nginx -t printed warnings:\n%s", out)
	}
}

func TestProxyCacheRequiresSafeCacheDir(t *testing.T) {
	svc := &Service{SnippetsDir: t.TempDir(), HTTPDir: t.TempDir(), CacheDir: "/var/cache/x; evil", Writer: &snippet.Writer{Tester: &countingTester{}}}
	if err := svc.Ensure(context.Background(), []string{"proxy-cache"}); err == nil {
		t.Error("expected an unsafe cache directory to be rejected")
	}
	svc.CacheDir = "/var/cache/nginx/gonix"
	if err := svc.Ensure(context.Background(), []string{"proxy-cache"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(svc.HTTPPath("proxy-cache"))
	if !strings.Contains(string(data), "proxy_cache_path /var/cache/nginx/gonix ") {
		t.Errorf("cache dir not substituted:\n%s", data)
	}
}

// TestProxyCacheBehavior checks with a real Nginx that a cacheable response
// is served from cache, while a request with a cookie reaches the backend.
func TestProxyCacheBehavior(t *testing.T) {
	p := nginxtest.New(t)
	hits := 0
	backend := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Cache-Control", "public, max-age=60")
		fmt.Fprintf(w, "hit %d", hits)
	})}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go backend.Serve(l)
	t.Cleanup(func() { backend.Close() })

	snippets := filepath.Join(p.Dir, "snippets")
	os.MkdirAll(snippets, 0o755)
	svc := &Service{SnippetsDir: snippets, HTTPDir: p.HTTPDir, CacheDir: filepath.Join(p.Dir, "cache"), Writer: &snippet.Writer{Tester: &countingTester{}}}
	if err := svc.Ensure(context.Background(), []string{"proxy-cache"}); err != nil {
		t.Fatal(err)
	}
	r, _ := nginx.NewRenderer(p.Dir, p.Dir, p.Dir, snippets)
	_, port, _ := net.SplitHostPort(l.Addr().String())
	portN, _ := strconv.Atoi(port)
	site, err := r.Render(nginx.Host{ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80, Presets: []string{"proxy-cache"},
		Locations: []nginx.Location{{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: portN, HTTPVersion: "1.1", BufferingEnabled: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	p.AddSite(t, "example.com", site)
	p.Start(t)

	get := func(cookie string) string {
		req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(p.Port(t, 80))+"/page", nil)
		req.Host = "example.com"
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if first, second := get(""), get(""); first != "hit 1" || second != "hit 1" {
		t.Errorf("cacheable response not cached: %q then %q", first, second)
	}
	if withCookie := get("session=abc"); withCookie != "hit 2" {
		t.Errorf("request with a cookie must reach the backend, got %q", withCookie)
	}
}
