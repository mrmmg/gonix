package nginx

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginxtest"
)

func redirectHost() Host {
	return Host{
		ServerName:  "domain.net",
		ServerAlias: []string{"www.domain.net"},
		Mode:        ModeRedirect,
		Listen:      80,
		ListenSSL:   443,
		AccessLog:   true,
		ErrorLog:    true,
		Redirect:    RedirectConfig{Target: "https://domain.com", Code: 301, PreservePath: true},
	}
}

func TestRenderRedirectHost(t *testing.T) {
	r := newTestRenderer(t)
	out, err := r.Render(redirectHost())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		"server_name domain.net www.domain.net;",
		"return 301 https://domain.com$request_uri;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered config missing %q\n--- output ---\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"root ", "location ", "proxy_pass"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("redirect host must not render %q\n--- output ---\n%s", unwanted, out)
		}
	}
}

func TestRenderRedirectHostWithSSLSkipsForceHTTPSHop(t *testing.T) {
	r := newTestRenderer(t)
	h := redirectHost()
	h.SSL = SSLConfig{Enabled: true, CertFile: "/c/fullchain.pem", KeyFile: "/c/privkey.pem", ForceHTTPS: true}
	out, err := r.Render(h)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out, `$scheme != "https"`) {
		t.Errorf("redirect host must redirect HTTP straight to the target, without a same-host HTTPS hop:\n%s", out)
	}
	if !strings.Contains(out, "listen 443 ssl;") {
		t.Errorf("missing SSL listener:\n%s", out)
	}
}

func TestRedirectHostRoundTrip(t *testing.T) {
	r := newTestRenderer(t)
	for _, tc := range []RedirectConfig{
		{Target: "https://domain.com", Code: 301, PreservePath: true},
		{Target: "https://domain.com/landing", Code: 302, PreservePath: false},
		{Target: "http://10.0.0.1:8080", Code: 308, PreservePath: true},
	} {
		h := redirectHost()
		h.Redirect = tc
		out, err := r.Render(h)
		if err != nil {
			t.Fatalf("Render(%+v): %v", tc, err)
		}
		parsed, err := ParseManagedHost(out)
		if err != nil {
			t.Fatalf("ParseManagedHost: %v", err)
		}
		if parsed.Mode != ModeRedirect {
			t.Errorf("Mode = %q, want %q", parsed.Mode, ModeRedirect)
		}
		if parsed.Redirect != tc {
			t.Errorf("Redirect = %+v, want %+v", parsed.Redirect, tc)
		}
		if len(parsed.ServerAlias) != 1 || parsed.ServerAlias[0] != "www.domain.net" {
			t.Errorf("ServerAlias = %v, want [www.domain.net]", parsed.ServerAlias)
		}
	}
}

func TestRedirectPreservePathTrimsTrailingSlash(t *testing.T) {
	got := RedirectConfig{Target: "https://domain.com/", PreservePath: true}.ReturnURL()
	if got != "https://domain.com$request_uri" {
		t.Errorf("ReturnURL = %q", got)
	}
}

func TestValidateRedirect(t *testing.T) {
	valid := []string{"https://domain.com", "http://domain.com/path", "https://10.0.0.1:8443"}
	for _, v := range valid {
		if err := ValidateRedirectTarget(v); err != nil {
			t.Errorf("ValidateRedirectTarget(%q) = %v, want nil", v, err)
		}
	}
	invalid := []string{
		"", "domain.com", "ftp://domain.com", "https://",
		"https://domain.com; return 200", "https://domain.com/$host", "https://x.com/{a}",
		"https://domain.com\"",
	}
	for _, v := range invalid {
		if err := ValidateRedirectTarget(v); err == nil {
			t.Errorf("ValidateRedirectTarget(%q) = nil, want error", v)
		}
	}

	h := redirectHost()
	h.Redirect.Code = 200
	if err := h.Validate(); err == nil {
		t.Error("expected an error for a non-redirect status code")
	}
}

func TestRedirectHostPassesRealNginx(t *testing.T) {
	p := nginxtest.New(t)
	r := newTestRenderer(t)
	h := redirectHost()
	cert, key := p.SelfSignedCert(t, "domain.net")
	h.SSL = SSLConfig{Enabled: true, CertFile: cert, KeyFile: key, ForceHTTPS: true}
	h.AccessLog, h.ErrorLog = false, false
	out, err := r.Render(h)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	p.AddSite(t, "domain.net", out)
	p.Test(t)
}

func newTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer("/var/log/nginx", "/var/log/nginx", "/etc/gonix/accesslists", "/etc/nginx/snippets")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return r
}

// TestRedirectBehavior sends a real request through Nginx.
func TestRedirectBehavior(t *testing.T) {
	p := nginxtest.New(t)
	r := newTestRenderer(t)
	h := redirectHost()
	h.Redirect.Code = 308
	out, err := r.Render(h)
	if err != nil {
		t.Fatal(err)
	}
	p.AddSite(t, "domain.net", strings.ReplaceAll(out, "/var/log/nginx/", p.Dir+"/"))
	p.Start(t)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(p.Port(t, 80))+"/blog/post?id=5", nil)
	req.Host = "www.domain.net"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 308 || resp.Header.Get("Location") != "https://domain.com/blog/post?id=5" {
		t.Errorf("got %d Location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}
