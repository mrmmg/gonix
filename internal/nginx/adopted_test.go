package nginx

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// certbotSite is a hand-written reverse proxy after `certbot --nginx`.
const certbotSite = `# My app, written by hand.
server {
    server_name example.com www.example.com;

    root /var/www/example;
    index index.html;
    add_header X-Frame-Options DENY;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_read_timeout 120s;
    }

    location ~ \.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/php8.2-fpm.sock;
    }

    listen [::]:443 ssl ipv6only=on; # managed by Certbot
    listen 443 ssl; # managed by Certbot
    ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem; # managed by Certbot
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem; # managed by Certbot
    include /etc/letsencrypt/options-ssl-nginx.conf; # managed by Certbot
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem; # managed by Certbot
}
server {
    if ($host = www.example.com) {
        return 301 https://$host$request_uri;
    } # managed by Certbot


    if ($host = example.com) {
        return 301 https://$host$request_uri;
    } # managed by Certbot


    listen 80;
    listen [::]:80;
    server_name example.com www.example.com;
    return 404; # managed by Certbot
}
`

// plainSite listens on both ports in one block and uses tabs.
const plainSite = "server {\n\tlisten 80;\n\tlisten 443 ssl;\n\tserver_name app.example.org;\n\n" +
	"\tif ($scheme != \"https\") {\n\t\treturn 301 https://$host$request_uri;\n\t}\n\n" +
	"\tssl_certificate /etc/ssl/app.pem;\n\tssl_certificate_key /etc/ssl/app.key;\n" +
	"\trewrite ^/old/(.*)$ /new/$1 permanent;\n\n" +
	"\tlocation / {\n\t\tproxy_pass http://backend:8080;\n\t}\n}\n"

func testRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer("/var/log/nginx", "/var/log/nginx", "/etc/gonix/accesslists", "/etc/nginx/snippets")
	if err != nil {
		t.Fatal(err)
	}
	r.Build = BuildInfo{Major: 1, Minor: 26, Patch: 0}
	return r
}

func adopt(t *testing.T, r *Renderer, content string) Host {
	t.Helper()
	h, err := r.ParseAdoptedHost("site.conf", AdoptContent(content))
	if err != nil {
		t.Fatalf("ParseAdoptedHost: %v", err)
	}
	return h
}

func render(t *testing.T, r *Renderer, h Host) string {
	t.Helper()
	out, err := r.Render(h)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

func TestAdoptMarker(t *testing.T) {
	c := AdoptContent(certbotSite)
	if !IsAdopted(c) || IsAdopted(certbotSite) {
		t.Fatal("IsAdopted")
	}
	back, err := ReleaseContent(c)
	if err != nil || back != certbotSite {
		t.Fatalf("ReleaseContent = %q, %v", back, err)
	}
	if strings.Contains(c, ManagedMarker) {
		t.Fatal("an adopted file must not look like one GoNix rendered")
	}
}

func TestInspectCertbotSite(t *testing.T) {
	r := testRenderer(t)
	rep, err := r.InspectAdoption("example.com", certbotSite)
	if err != nil {
		t.Fatal(err)
	}
	h := rep.Host
	if h.ServerName != "example.com" || !slices.Equal(h.ServerAlias, []string{"www.example.com"}) {
		t.Errorf("names = %s %v", h.ServerName, h.ServerAlias)
	}
	if !rep.HTTPBlock || h.Listen != 80 || h.ListenSSL != 443 || !h.SSL.Enabled || !h.SSL.ForceHTTPS {
		t.Errorf("layout: http block %v, listen %d/%d, ssl %+v", rep.HTTPBlock, h.Listen, h.ListenSSL, h.SSL)
	}
	if h.SSL.CertFile != "/etc/letsencrypt/live/example.com/fullchain.pem" || h.SSL.KeyFile != "/etc/letsencrypt/live/example.com/privkey.pem" {
		t.Errorf("certificate = %+v", h.SSL)
	}
	if h.Mode != ModeReverseProxy || len(h.Locations) != 2 {
		t.Fatalf("mode %s, locations %+v", h.Mode, h.Locations)
	}
	p := h.Locations[0].Proxy
	if p == nil || p.UpstreamURL() != "http://127.0.0.1:3000" || !p.ForwardHost || !p.ForwardFor || p.ForwardProto || p.ReadTimeout != "120s" || p.HTTPVersion != "1.1" {
		t.Errorf("proxy / = %+v", p)
	}
	if l := h.Locations[1]; !l.IsCustom || l.Path != `~ \.php$` || !strings.Contains(l.CustomConfig, "fastcgi_pass") {
		t.Errorf("php location = %+v", l)
	}
	want := []string{"index index.html", "add_header X-Frame-Options DENY", "include /etc/letsencrypt/options-ssl-nginx.conf", "ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem"}
	if !slices.Equal(rep.Kept, want) {
		t.Errorf("kept = %q", rep.Kept)
	}
	if !slices.Equal(rep.Custom, []string{`~ \.php$`}) {
		t.Errorf("custom = %q", rep.Custom)
	}
}

func TestInspectRejects(t *testing.T) {
	r := testRenderer(t)
	for name, content := range map[string]string{
		"no server":     "upstream app { server 127.0.0.1:1; }\n",
		"three servers": "server { listen 80; } server { listen 81; } server { listen 82; }\n",
		"two sites":     "server { listen 80; server_name a.com; } server { listen 80; server_name b.com; }\n",
		"syntax":        "server { listen 80 }\n",
	} {
		if _, err := r.InspectAdoption("f", content); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestAdoptedUnchangedRendersIdentically(t *testing.T) {
	r := testRenderer(t)
	for _, src := range []string{certbotSite, plainSite} {
		h := adopt(t, r, src)
		if got := render(t, r, h); got != AdoptContent(src) {
			t.Errorf("unchanged host re-rendered differently:\n%s", got)
		}
	}
}

// change applies edit to a freshly adopted host and returns the new file.
func change(t *testing.T, r *Renderer, src string, edit func(h *Host)) string {
	t.Helper()
	h := adopt(t, r, src)
	edit(&h)
	return render(t, r, h)
}

// roundTrip applies on, then off, and expects the original file back.
func roundTrip(t *testing.T, r *Renderer, src string, on, off func(h *Host)) string {
	t.Helper()
	changed := change(t, r, src, on)
	h, err := r.ParseAdoptedHost("site.conf", changed)
	if err != nil {
		t.Fatal(err)
	}
	off(&h)
	if back := render(t, r, h); back != AdoptContent(src) {
		t.Errorf("undoing the change did not restore the file:\n%s", back)
	}
	return changed
}

func TestAdoptedAliasesFollowCertbotBlock(t *testing.T) {
	r := testRenderer(t)
	out := change(t, r, certbotSite, func(h *Host) { h.ServerAlias = []string{"api.example.com"} })
	if strings.Count(out, "server_name example.com api.example.com;") != 2 {
		t.Errorf("both blocks should list the new names:\n%s", out)
	}
	if !strings.Contains(out, "if ($host = api.example.com) {") || strings.Contains(out, "if ($host = www.example.com)") {
		t.Errorf("certbot's per-name redirects not updated:\n%s", out)
	}
	if !strings.Contains(out, "if ($host = example.com) {\n        return 301 https://$host$request_uri;\n    } # managed by Certbot") {
		t.Errorf("an untouched redirect lost its formatting:\n%s", out)
	}
}

func TestAdoptedSwitchToGoNixCertificate(t *testing.T) {
	r := testRenderer(t)
	out := change(t, r, certbotSite, func(h *Host) {
		h.SSL.CertFile = "/etc/nginx/certs/example.com/fullchain.pem"
		h.SSL.KeyFile = "/etc/nginx/certs/example.com/privkey.pem"
	})
	if !strings.Contains(out, "    ssl_certificate /etc/nginx/certs/example.com/fullchain.pem;\n    ssl_certificate_key /etc/nginx/certs/example.com/privkey.pem;\n    include /etc/letsencrypt/options-ssl-nginx.conf; # managed by Certbot") {
		t.Errorf("certificate lines not switched in place:\n%s", out)
	}
	if strings.Contains(out, "privkey.pem; # managed by Certbot") {
		t.Error(`"managed by Certbot" left on a line certbot no longer manages`)
	}
}

func TestAdoptedRefusals(t *testing.T) {
	r := testRenderer(t)
	for name, tc := range map[string]struct {
		src  string
		edit func(h *Host)
	}{
		"ssl off":     {certbotSite, func(h *Host) { h.SSL = SSLConfig{} }},
		"rename":      {certbotSite, func(h *Host) { h.ServerName = "other.com" }},
		"force-https": {certbotSite, func(h *Host) { h.SSL.ForceHTTPS = false }},
		"http3":       {certbotSite, func(h *Host) { h.SSL.HTTP3 = true }},
		"port":        {plainSite, func(h *Host) { h.Listen = 8080 }},
		"mode":        {plainSite, func(h *Host) { h.Mode = ModeStatic; h.Root = "/srv" }},
		"bad custom":  {certbotSite, func(h *Host) { h.Locations[1].CustomConfig = "fastcgi_pass x" }},
	} {
		h := adopt(t, r, tc.src)
		tc.edit(&h)
		if out, err := r.Render(h); err == nil {
			t.Errorf("%s: expected a refusal, got\n%s", name, out)
		}
	}
}

// A custom body GoNix reads back as a plain proxy is the same location.
func TestAdoptedCustomThatReadsAsProxy(t *testing.T) {
	r := testRenderer(t)
	out := change(t, r, plainSite, func(h *Host) {
		h.Locations = append(h.Locations, Location{Path: "/x/", IsCustom: true, CustomConfig: "proxy_pass http://127.0.0.1:9;"})
	})
	if !strings.Contains(out, "\tlocation /x/ {\n\t\tproxy_pass http://127.0.0.1:9;\n\t}") {
		t.Errorf("got\n%s", out)
	}
}

func TestAdoptedHTTP2(t *testing.T) {
	r := testRenderer(t)
	out := roundTrip(t, r, plainSite, func(h *Host) { h.SSL.HTTP2 = true }, func(h *Host) { h.SSL.HTTP2 = false })
	if !strings.Contains(out, "\tserver_name app.example.org;\n\thttp2 on;\n") {
		t.Errorf("http2 on not added after server_name:\n%s", out)
	}
	r.Build = BuildInfo{Major: 1, Minor: 18}
	out = roundTrip(t, r, plainSite, func(h *Host) { h.SSL.HTTP2 = true }, func(h *Host) { h.SSL.HTTP2 = false })
	if !strings.Contains(out, "\tlisten 443 ssl http2;\n") {
		t.Errorf("old Nginx should get http2 on the listen line:\n%s", out)
	}
}

func TestAdoptedForceHTTPS(t *testing.T) {
	r := testRenderer(t)
	src := strings.Replace(plainSite, "\tif ($scheme != \"https\") {\n\t\treturn 301 https://$host$request_uri;\n\t}\n\n", "", 1)
	out := change(t, r, src, func(h *Host) { h.SSL.ForceHTTPS = true })
	if !strings.Contains(out, "\tssl_certificate_key /etc/ssl/app.key;\n\n\tif ($scheme != \"https\") {\n\t\treturn 301 https://$host$request_uri;\n\t}\n") {
		t.Errorf("redirect not added after the certificate:\n%s", out)
	}
	out = change(t, r, plainSite, func(h *Host) { h.SSL.ForceHTTPS = false })
	if strings.Contains(out, "$scheme") {
		t.Errorf("redirect not removed:\n%s", out)
	}
}

func TestAdoptedEnableSSL(t *testing.T) {
	r := testRenderer(t)
	src := "server {\n    listen 80;\n    server_name a.example.com;\n    location / {\n        proxy_pass http://127.0.0.1:1;\n    }\n}\n"
	out := change(t, r, src, func(h *Host) {
		h.ListenSSL = 443
		h.SSL = SSLConfig{Enabled: true, CertFile: "/c.pem", KeyFile: "/k.pem", HTTP2: true, ForceHTTPS: true}
	})
	want := "    listen 80;\n    listen 443 ssl;\n    server_name a.example.com;\n    ssl_certificate /c.pem;\n    ssl_certificate_key /k.pem;\n    http2 on;\n\n" +
		"    if ($scheme != \"https\") {\n        return 301 https://$host$request_uri;\n    }\n    location / {"
	if !strings.Contains(out, want) {
		t.Errorf("got\n%s", out)
	}
}

func TestAdoptedSnippetsRoundTrip(t *testing.T) {
	r := testRenderer(t)
	edits := map[string]func(h *Host, on bool){
		"presets": func(h *Host, on bool) {
			if on {
				h.Presets = []string{"security-headers", "gzip"}
			} else {
				h.Presets = nil
			}
		},
		"ip list":     func(h *Host, on bool) { h.IPListName = map[bool]string{true: "office"}[on] },
		"rate limit":  func(h *Host, on bool) { h.RateLimit = map[bool]string{true: "api"}[on] },
		"country":     func(h *Host, on bool) { h.CountryList = map[bool]string{true: "IR"}[on] },
		"error pages": func(h *Host, on bool) { h.ErrorPagesSnippet = map[bool]string{true: "default"}[on] },
		"access list": func(h *Host, on bool) { h.AccessListName = map[bool]string{true: "admins"}[on]; h.SatisfyAny = on },
		"maintenance": func(h *Host, on bool) { h.Maintenance = on },
		"aliases":     func(h *Host, on bool) { h.ServerAlias = map[bool][]string{true: {"b.example.com"}}[on] },
	}
	for name, edit := range edits {
		for _, src := range []string{certbotSite, plainSite} {
			if name == "aliases" && src == certbotSite {
				continue // certbot's per-name redirects are re-created, not restored
			}
			roundTrip(t, r, src, func(h *Host) { edit(h, true) }, func(h *Host) { edit(h, false) })
		}
	}
}

func TestAdoptedRewritePoint(t *testing.T) {
	r := testRenderer(t)
	out := change(t, r, plainSite, func(h *Host) { h.Maintenance = true; h.CountryList = "IR" })
	scheme := strings.Index(out, "$scheme")
	maint := strings.Index(out, "set $gonix_maintenance")
	country := strings.Index(out, "gonix-geo-IR.conf")
	rewrite := strings.Index(out, "rewrite ^/old")
	if !(scheme < maint && maint < country && country < rewrite) {
		t.Errorf("order: scheme %d, maintenance %d, country %d, rewrite %d\n%s", scheme, maint, country, rewrite, out)
	}
}

func TestAdoptedLocations(t *testing.T) {
	r := testRenderer(t)
	out := change(t, r, certbotSite, func(h *Host) {
		p := *h.Locations[0].Proxy
		p.UpstreamPort, p.WebSocket, p.ReadTimeout, p.ForwardProto = 4000, true, "", true
		h.Locations[0].Proxy = &p
		h.Locations = append(h.Locations, Location{Path: "/api/", IPListName: "office", Proxy: &ProxyConfig{
			UpstreamScheme: "http", UpstreamHost: "10.0.0.2", UpstreamPort: 9000, HTTPVersion: "1.1", BufferingEnabled: true, ForwardHost: true}})
	})
	want := `    location / {
        proxy_pass http://127.0.0.1:4000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection upgrade;
    }`
	if !strings.Contains(out, want) {
		t.Errorf("location / not patched in place:\n%s", out)
	}
	want = `        fastcgi_pass unix:/run/php/php8.2-fpm.sock;
    }

    location /api/ {
        proxy_pass http://10.0.0.2:9000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        include /etc/nginx/snippets/gonix-ip-office.conf;
    }

    listen [::]:443 ssl ipv6only=on; # managed by Certbot`
	if !strings.Contains(out, want) {
		t.Errorf("new location not added after the last one:\n%s", out)
	}

	out = change(t, r, certbotSite, func(h *Host) { h.Locations = h.Locations[1:] })
	if strings.Contains(out, "proxy_pass") || !strings.Contains(out, "fastcgi_pass") {
		t.Errorf("removing / went wrong:\n%s", out)
	}
	out = change(t, r, plainSite, func(h *Host) {
		h.Locations = append(h.Locations, Location{Path: "/static/", IsCustom: true, CustomConfig: "        alias /srv/static/;\n        expires 7d;\n"})
	})
	if !strings.Contains(out, "\tlocation /static/ {\n\t\talias /srv/static/;\n\t\texpires 7d;\n\t}\n}") {
		t.Errorf("custom location:\n%s", out)
	}
}

func TestAdoptedCustomLocationEdit(t *testing.T) {
	r := testRenderer(t)
	h := adopt(t, r, certbotSite)
	h.Locations[1].CustomConfig = strings.Replace(h.Locations[1].CustomConfig, "php8.2", "php8.3", 1)
	out := render(t, r, h)
	if want := strings.Replace(AdoptContent(certbotSite), "php8.2", "php8.3", 1); out != want {
		t.Errorf("custom edit changed more than asked:\n%s", out)
	}
}

func TestAdoptedRedirectHost(t *testing.T) {
	r := testRenderer(t)
	src := "server {\n    listen 80;\n    server_name old.example.com;\n    return 301 https://new.example.com$request_uri;\n}\n"
	h := adopt(t, r, src)
	if h.Mode != ModeRedirect || h.Redirect.Target != "https://new.example.com" || !h.Redirect.PreservePath || h.Redirect.Code != 301 {
		t.Fatalf("redirect = %s %+v", h.Mode, h.Redirect)
	}
	h.Redirect.Code, h.Redirect.PreservePath = 308, false
	if out := render(t, r, h); !strings.Contains(out, "    return 308 https://new.example.com;\n") {
		t.Errorf("got\n%s", out)
	}
	h.Maintenance = true
	if _, err := r.Render(h); err == nil {
		t.Error("maintenance on a redirect host should be refused")
	}
}

func TestAdoptedForeignAuthIsNotGoNixs(t *testing.T) {
	r := testRenderer(t)
	src := "server {\n    listen 80;\n    server_name a.example.com;\n    auth_basic \"Team\";\n    auth_basic_user_file /etc/nginx/.htpasswd;\n    root /srv;\n}\n"
	rep, err := r.InspectAdoption("a", src)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Host.AccessListName != "" || !slices.Contains(rep.Kept, "auth_basic \"Team\"") {
		t.Errorf("foreign Basic Auth mapped: %+v kept %q", rep.Host.AccessListName, rep.Kept)
	}
	out := change(t, r, src, func(h *Host) { h.AccessListName = "admins" })
	if !strings.Contains(out, "    auth_basic \"Team\";\n    auth_basic_user_file /etc/gonix/accesslists/admins.htpasswd;\n") {
		t.Errorf("got\n%s", out)
	}
}

// Logging off replaces every log directive of the block with one that
// discards; back on, the host logs to its own files like any GoNix host.
func TestAdoptedLogs(t *testing.T) {
	r := testRenderer(t)
	src := "server {\n    listen 80;\n    server_name a.example.com;\n    access_log /var/log/a.log main;\n    access_log syslog:server=10.0.0.1;\n    root /srv;\n}\n"
	h := adopt(t, r, src)
	if !h.AccessLog || !h.ErrorLog {
		t.Fatalf("logs = %v %v", h.AccessLog, h.ErrorLog)
	}
	h.AccessLog, h.ErrorLog = false, false
	out := render(t, r, h)
	if !strings.Contains(out, "    server_name a.example.com;\n    access_log off;\n    root /srv;\n    error_log /dev/null;\n") {
		t.Fatalf("got\n%s", out)
	}
	h, _ = r.ParseAdoptedHost("site.conf", out)
	if h.AccessLog || h.ErrorLog {
		t.Fatalf("read back logs = %v %v", h.AccessLog, h.ErrorLog)
	}
	h.AccessLog = true
	if out = render(t, r, h); !strings.Contains(out, "    access_log /var/log/nginx/a.example.com.access.log;\n") {
		t.Errorf("got\n%s", out)
	}
	// One `access_log off` cancels the others of its level.
	if h := adopt(t, r, strings.Replace(src, "    root", "    access_log off;\n    root", 1)); h.AccessLog {
		t.Error("access_log off next to another log should read as off")
	}
}

func TestManagerAdoptedHosts(t *testing.T) {
	r := testRenderer(t)
	avail, enabled := t.TempDir(), t.TempDir()
	m := NewManager(avail, enabled, r)
	path := avail + "/hand.conf"
	src := AdoptContent("server {\n    listen 80;\n    server_name hand.example.com;\n    root /srv;\n}\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := m.ListHosts()
	if err != nil || len(list) != 1 || !list[0].Managed || list[0].Adopted == nil || list[0].Mode != ModeStatic {
		t.Fatalf("ListHosts = %+v, %v", list, err)
	}
	h, err := m.GetHost("hand.conf")
	if err != nil || h.ConfigFile() != "hand.conf" {
		t.Fatalf("GetHost = %+v, %v", h, err)
	}

	// A hand edit after the host was read is never overwritten.
	edited := strings.Replace(src, "/srv", "/srv/new", 1)
	os.WriteFile(path, []byte(edited), 0o644)
	h.Root = "/other"
	if err := m.WriteHost(h); err == nil || !strings.Contains(err.Error(), "changed since it was read") {
		t.Fatalf("stale write: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != edited {
		t.Fatal("stale write changed the file")
	}
	h, _ = m.GetHost("hand.conf")
	h.Root = "/other"
	if err := m.WriteHost(h); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "    root /other;\n") {
		t.Fatalf("got\n%s", data)
	}

	// Broken by hand: listed read-only, with the reason.
	os.WriteFile(path, []byte(src+"server { listen 81; }\nserver { listen 82; }\n"), 0o644)
	list, _ = m.ListHosts()
	if list[0].Managed || list[0].AdoptError == "" || list[0].ServerName != "hand.example.com" {
		t.Fatalf("broken adopted file listed as %+v", list[0])
	}
	if _, err := m.GetHost("hand.conf"); err == nil {
		t.Fatal("GetHost on a broken adopted file should fail")
	}
}

func TestAdoptedReviewFixes(t *testing.T) {
	r := testRenderer(t)
	// Only the code changes: the hand-written target is not re-validated.
	src := "server {\n    listen 80;\n    server_name a.example.com;\n    return 301 https://$host$request_uri;\n}\n"
	if out := change(t, r, src, func(h *Host) { h.Redirect.Code = 308 }); !strings.Contains(out, "return 308 https://$host$request_uri;") {
		t.Errorf("got\n%s", out)
	}
	// HTTP/2 off leaves a plain-HTTP (h2c) listen alone.
	src = "server {\n    listen 8080 http2;\n    listen 443 ssl http2;\n    server_name a.example.com;\n    ssl_certificate /c;\n    ssl_certificate_key /k;\n    root /srv;\n}\n"
	if out := change(t, r, src, func(h *Host) { h.SSL.HTTP2 = false }); !strings.Contains(out, "listen 8080 http2;\n    listen 443 ssl;\n") {
		t.Errorf("got\n%s", out)
	}
	// A one-line custom edit still gets its own lines.
	out := change(t, r, certbotSite, func(h *Host) { h.Locations[1].CustomConfig = "fastcgi_pass unix:/x.sock;" })
	if !strings.Contains(out, "    location ~ \\.php$ {\n        fastcgi_pass unix:/x.sock;\n    }\n") {
		t.Errorf("got\n%s", out)
	}
}

func TestAdoptedHTTPBlockForOtherNamesIsLeftAlone(t *testing.T) {
	r := testRenderer(t)
	src := "server {\n    listen 443 ssl;\n    server_name example.com;\n    ssl_certificate /c;\n    ssl_certificate_key /k;\n    root /srv;\n}\n" +
		"server {\n    listen 80;\n    server_name www.example.com example.com;\n    return 301 https://example.com$request_uri;\n}\n"
	out := change(t, r, src, func(h *Host) { h.ServerAlias = []string{"api.example.com"} })
	if !strings.Contains(out, "server_name example.com api.example.com;") || !strings.Contains(out, "server_name www.example.com example.com;") {
		t.Errorf("got\n%s", out)
	}
}
