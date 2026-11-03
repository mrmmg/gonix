package nginx

import (
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginxtest"
)

func TestRenderReverseProxyHost(t *testing.T) {
	r, err := NewRenderer("/var/log/nginx", "/var/log/nginx", "/etc/gonix/accesslists", "/etc/nginx/snippets")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	h := Host{
		ServerName: "example.com",
		Mode:       ModeReverseProxy,
		Listen:     80,
		ListenSSL:  443,
		SSL: SSLConfig{
			Enabled:    true,
			CertFile:   "/etc/nginx/certs/example.com/fullchain.pem",
			KeyFile:    "/etc/nginx/certs/example.com/privkey.pem",
			HTTP2:      true,
			ForceHTTPS: true,
		},
		AccessLog: true,
		ErrorLog:  true,
		Locations: []Location{
			{
				Path: "/",
				Proxy: &ProxyConfig{
					UpstreamScheme: "http",
					UpstreamHost:   "127.0.0.1",
					UpstreamPort:   8080,
					HTTPVersion:    "1.1",
					ForwardHost:    true,
					ForwardFor:     true,
					ForwardProto:   true,
					WebSocket:      true,
					ConnectTimeout: "60s",
					ReadTimeout:    "60s",
					SendTimeout:    "60s",
				},
			},
		},
	}

	out, err := r.Render(h)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	for _, want := range []string{
		"server_name example.com;",
		"listen 80;",
		"listen 443 ssl http2;",
		"ssl_certificate /etc/nginx/certs/example.com/fullchain.pem;",
		"ssl_certificate_key /etc/nginx/certs/example.com/privkey.pem;",
		"proxy_pass http://127.0.0.1:8080;",
		"proxy_http_version 1.1;",
		"proxy_set_header Upgrade $http_upgrade;",
		"proxy_set_header Connection \"upgrade\";",
		"access_log /var/log/nginx/example.com.access.log;",
		"error_log /var/log/nginx/example.com.error.log;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered config missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestRenderCustomLocation(t *testing.T) {
	r, err := NewRenderer("/var/log/nginx", "/var/log/nginx", "/etc/gonix/accesslists", "/etc/nginx/snippets")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	h := Host{
		ServerName: "custom.example.com",
		Mode:       ModeReverseProxy,
		Listen:     80,
		AccessLog:  true,
		ErrorLog:   true,
		Locations: []Location{
			{Path: "/custom/", IsCustom: true, CustomConfig: "        return 200 'ok';"},
		},
	}
	out, err := r.Render(h)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "location /custom/ {") {
		t.Errorf("missing custom location block:\n%s", out)
	}
	if !strings.Contains(out, "return 200 'ok';") {
		t.Errorf("missing custom directive:\n%s", out)
	}
}

func TestHostValidate(t *testing.T) {
	h := Host{Mode: ModeReverseProxy}
	if err := h.Validate(); err == nil {
		t.Fatal("expected error for empty server name")
	}
	h.ServerName = "example.com"
	if err := h.Validate(); err == nil {
		t.Fatal("expected error for reverse proxy host with no locations")
	}
}

func TestRenderAndParseErrorPagesSnippet(t *testing.T) {
	r, err := NewRenderer("/var/log/nginx", "/var/log/nginx", "/etc/gonix/accesslists", "/etc/nginx/snippets")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	h := Host{
		ServerName:        "example.com",
		Mode:              ModeReverseProxy,
		Listen:            80,
		AccessLog:         true,
		ErrorLog:          true,
		ErrorPagesSnippet: "default",
		Locations: []Location{
			{Path: "/", Proxy: &ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}},
		},
	}
	out, err := r.Render(h)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "include /etc/nginx/snippets/gonix-error-pages-default.conf;") {
		t.Fatalf("missing error pages include:\n%s", out)
	}

	parsed, err := ParseManagedHost(out)
	if err != nil {
		t.Fatalf("ParseManagedHost: %v", err)
	}
	if parsed.ErrorPagesSnippet != "default" {
		t.Errorf("ErrorPagesSnippet = %q, want %q", parsed.ErrorPagesSnippet, "default")
	}

	h.ErrorPagesSnippet = ""
	out, err = r.Render(h)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out, "include") {
		t.Errorf("unexpected include without a snippet:\n%s", out)
	}

	h.ErrorPagesSnippet = "../evil"
	if _, err := r.Render(h); err == nil {
		t.Error("expected an error for an invalid snippet name")
	}
}

func TestReverseProxyHostPassesRealNginx(t *testing.T) {
	p := nginxtest.New(t)
	r := newTestRenderer(t)
	cert, key := p.SelfSignedCert(t, "example.com")
	h := Host{
		ServerName: "example.com",
		Mode:       ModeReverseProxy,
		Listen:     80,
		ListenSSL:  443,
		SSL:        SSLConfig{Enabled: true, CertFile: cert, KeyFile: key, HTTP2: true, ForceHTTPS: true},
		Locations: []Location{
			{Path: "/", Proxy: &ProxyConfig{
				UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1",
				ForwardHost: true, ForwardFor: true, ForwardProto: true, WebSocket: true,
				ConnectTimeout: "60s", ReadTimeout: "60s", SendTimeout: "60s", MaxBodySize: "10m",
			}},
			{Path: "/custom/", IsCustom: true, CustomConfig: "        return 200 'ok';"},
		},
	}
	out, err := r.Render(h)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	p.AddSite(t, "example.com", out)
	p.Test(t)
}

// Regression test: location-level access lists used to be mistaken for
// host-level ones, because the server-level section of the file was never
// cut at the first location block.
func TestParseLocationAccessListIsNotHostLevel(t *testing.T) {
	r := newTestRenderer(t)
	h := Host{
		ServerName: "example.com", Mode: ModeReverseProxy, Listen: 80,
		Locations: []Location{{Path: "/admin/", AccessListName: "admins", Proxy: &ProxyConfig{
			UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}}},
	}
	out, err := r.Render(h)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseManagedHost(out)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.AccessListName != "" {
		t.Errorf("host-level AccessListName = %q, want empty", parsed.AccessListName)
	}
	if parsed.Locations[0].AccessListName != "admins" {
		t.Errorf("location AccessListName = %q, want admins", parsed.Locations[0].AccessListName)
	}
}

// Regression test: `error_log off;` makes Nginx log to a file named "off".
func TestDisabledErrorLogUsesDevNull(t *testing.T) {
	r := newTestRenderer(t)
	h := Host{ServerName: "example.com", Mode: ModeStatic, Root: "/srv", Listen: 80, AccessLog: false, ErrorLog: false}
	out, err := r.Render(h)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "error_log off;") || !strings.Contains(out, "error_log /dev/null;") {
		t.Errorf("disabled error log must render /dev/null:\n%s", out)
	}
	for _, content := range []string{out, strings.Replace(out, "error_log /dev/null;", "error_log off;", 1)} {
		parsed, err := ParseManagedHost(content)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.ErrorLog || parsed.AccessLog {
			t.Errorf("logs should parse as disabled: access=%v error=%v", parsed.AccessLog, parsed.ErrorLog)
		}
	}
	h.ErrorLog = true
	out, _ = r.Render(h)
	if parsed, _ := ParseManagedHost(out); !parsed.ErrorLog {
		t.Error("enabled error log parsed as disabled")
	}
}
