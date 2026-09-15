package nginx

import (
	"strings"
	"testing"
)

func TestRenderReverseProxyHost(t *testing.T) {
	r, err := NewRenderer("/var/log/nginx", "/var/log/nginx", "/etc/nginx-manager/accesslists")
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
	r, err := NewRenderer("/var/log/nginx", "/var/log/nginx", "/etc/nginx-manager/accesslists")
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
