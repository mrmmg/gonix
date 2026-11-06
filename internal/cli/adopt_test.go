package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// certbotFixture writes a certbot directory with a lineage for name and a
// hand-written, certbot-edited site using it; it returns the site's
// content.
func certbotFixture(t *testing.T, h *harness, name string) string {
	t.Helper()
	le := t.TempDir()
	live := filepath.Join(le, "live", name)
	os.MkdirAll(live, 0o755)
	os.MkdirAll(filepath.Join(le, "renewal"), 0o755)
	os.WriteFile(filepath.Join(le, "renewal", name+".conf"),
		[]byte("[renewalparams]\nauthenticator = nginx\ninstaller = nginx\n"), 0o644)
	site := `server {
    server_name ` + name + ` *.` + name + `;
    root /var/www/app;
    try_files $uri @app;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
    }

    location @app {
        proxy_pass http://127.0.0.1:3000;
        proxy_intercept_errors on;
    }

    listen 443 ssl; # managed by Certbot
    ssl_certificate ` + live + `/fullchain.pem; # managed by Certbot
    ssl_certificate_key ` + live + `/privkey.pem; # managed by Certbot
    include /etc/letsencrypt/options-ssl-nginx.conf; # managed by Certbot
}
server {
    if ($host = ` + name + `) {
        return 301 https://$host$request_uri;
    } # managed by Certbot

    listen 80;
    server_name ` + name + ` *.` + name + `;
    return 404; # managed by Certbot
}
`
	path := filepath.Join(h.svc.Config.Nginx.SitesAvailable, name+".conf")
	if err := os.WriteFile(path, []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	return site
}

func TestAdoptLifecycle(t *testing.T) {
	h := newHarness(t)
	site := certbotFixture(t, h, "example.com")
	path := filepath.Join(h.svc.Config.Nginx.SitesAvailable, "example.com.conf")
	read := func() string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	if out := h.fails(1, "host", "set", "example.com", "--upstream", "127.0.0.1:4000"); !strings.Contains(out, "gonix adopt example.com") {
		t.Errorf("editing an unmanaged host should point to adopt:\n%s", out)
	}

	out := h.ok("adopt", "example.com", "--dry-run")
	if !strings.Contains(out, "+# Adopted by GoNix.") || !strings.Contains(out, "renewed by certbot") {
		t.Errorf("dry run:\n%s", out)
	}
	if read() != site {
		t.Fatal("dry run changed the file")
	}

	out = h.ok("adopt", "example.com")
	for _, want := range []string{"Adopted example.com", "renewed by certbot (authenticator nginx)", "try_files $uri @app", "custom locations (kept as written): @app"} {
		if !strings.Contains(out, want) {
			t.Errorf("adopt output lacks %q:\n%s", want, out)
		}
	}
	adopted := read()
	if adopted != "# Adopted by GoNix. GoNix edits only the settings it manages; everything\n# else in this file is kept exactly as written.\n"+site {
		t.Fatalf("adopting changed more than the header:\n%s", adopted)
	}

	var shown hostJSON
	json.Unmarshal([]byte(h.ok("host", "show", "example.com", "--json")), &shown)
	if !shown.Managed || !shown.Adopted || shown.Certbot == nil || shown.Certbot.Name != "example.com" || shown.SSL == nil || !shown.SSL.ForceHTTPS {
		t.Fatalf("host show = %+v", shown)
	}

	// Edits touch only their directives; the wildcard alias stays valid.
	h.ok("host", "set", "example.com", "--upstream", "127.0.0.1:4000")
	if want := strings.Replace(adopted, "proxy_pass http://127.0.0.1:3000;\n        proxy_set_header", "proxy_pass http://127.0.0.1:4000;\n        proxy_set_header", 1); read() != want {
		t.Fatalf("upstream change:\n%s", read())
	}
	before := read()
	h.ok("host", "set", "example.com", "--maintenance", "on")
	if !strings.Contains(read(), "set $gonix_maintenance $gonix_maint_example_com$uri;") {
		t.Fatalf("maintenance on:\n%s", read())
	}
	h.ok("host", "set", "example.com", "--maintenance", "off")
	if read() != before {
		t.Fatalf("maintenance off did not restore the file:\n%s", read())
	}
	h.ok("host", "path", "add", "example.com", "/api/", "--upstream", "http://127.0.0.1:5000")
	h.ok("host", "path", "remove", "example.com", "/api/")
	if read() != before {
		t.Fatalf("adding and removing a path did not restore the file:\n%s", read())
	}
	h.ok("host", "set", "example.com", "--alias", "www.example.com")
	if got := read(); strings.Count(got, "server_name example.com *.example.com www.example.com;") != 2 || !strings.Contains(got, "if ($host = www.example.com)") {
		t.Fatalf("alias:\n%s", got)
	}

	// Moving to a GoNix certificate keeps HTTP/2 off, as it was.
	h.ok("host", "set", "example.com", "--ssl", "example.com")
	got := read()
	certs := h.svc.Config.Certificates.Directory
	if !strings.Contains(got, "    ssl_certificate "+certs+"/example.com/fullchain.pem;\n    ssl_certificate_key "+certs+"/example.com/privkey.pem;\n") ||
		strings.Contains(got, "http2") {
		t.Fatalf("certificate switch:\n%s", got)
	}

	// Refused changes leave the file alone.
	before = read()
	if out := h.fails(1, "host", "set", "example.com", "--ssl", "none"); !strings.Contains(out, "not supported for an adopted host") {
		t.Errorf("turning SSL off:\n%s", out)
	}
	if read() != before {
		t.Fatal("a refused change was written")
	}

	h.ok("adopt", "release", "example.com")
	if got := read(); got != strings.TrimPrefix(before, "# Adopted by GoNix. GoNix edits only the settings it manages; everything\n# else in this file is kept exactly as written.\n") {
		t.Fatalf("release:\n%s", got)
	}
	var released hostJSON
	json.Unmarshal([]byte(h.ok("host", "show", "example.com", "--json")), &released)
	if released.Managed || released.Adopted {
		t.Errorf("released host = %+v", released)
	}
}

func TestAdoptAllSkipsWhatItCannotRead(t *testing.T) {
	h := newHarness(t)
	sites := h.svc.Config.Nginx.SitesAvailable
	os.WriteFile(filepath.Join(sites, "multi"), []byte("server { listen 80; server_name a.com; }\nserver { listen 80; server_name b.com; }\n"), 0o644)
	os.WriteFile(filepath.Join(sites, "default"), []byte("server { listen 80 default_server; server_name _; root /var/www/html; }\n"), 0o644)
	out := h.fails(1, "adopt", "--all")
	if !strings.Contains(out, "2 server blocks") || !strings.Contains(out, "must be a plain domain") {
		t.Errorf("adopt --all:\n%s", out)
	}
	os.WriteFile(filepath.Join(sites, "ok.example.com"), []byte("server {\n    listen 80;\n    server_name ok.example.com;\n    root /srv;\n}\n"), 0o644)
	out = h.ok("adopt", "--all")
	if !strings.Contains(out, "Adopted ok.example.com") || !strings.Contains(out, "Skipped multi") {
		t.Errorf("adopt --all:\n%s", out)
	}
	if out := h.ok("host", "list"); !strings.Contains(out, "static (adopted)") {
		t.Errorf("host list:\n%s", out)
	}
}

func TestAdoptedFileEditedByHandBecomesReadOnly(t *testing.T) {
	h := newHarness(t)
	sites := h.svc.Config.Nginx.SitesAvailable
	path := filepath.Join(sites, "a.example.com")
	os.WriteFile(path, []byte("server {\n    listen 80;\n    server_name a.example.com;\n    root /srv;\n}\n"), 0o644)
	h.ok("adopt", "a.example.com")
	data, _ := os.ReadFile(path)
	os.WriteFile(path, append(data, []byte("server { listen 80; server_name b.example.com; }\n")...), 0o644)
	if out := h.fails(1, "host", "set", "a.example.com", "--root", "/x"); !strings.Contains(out, "cannot be edited") {
		t.Errorf("got:\n%s", out)
	}
	if out := h.ok("host", "list"); !strings.Contains(out, "adopted, unreadable") {
		t.Errorf("host list:\n%s", out)
	}
}
