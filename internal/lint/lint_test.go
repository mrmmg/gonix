package lint

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrmmg/gonix/internal/nginx"
)

type env struct {
	c   *Checker
	mgr *nginx.Manager
}

func newEnv(t *testing.T) env {
	t.Helper()
	root := t.TempDir()
	dirs := map[string]string{}
	for _, d := range []string{"available", "enabled", "conf.d", "snippets", "accesslists", "etc"} {
		dirs[d] = filepath.Join(root, d)
		if err := os.MkdirAll(dirs[d], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	nginxConf := "http {\n    include " + dirs["conf.d"] + "/*.conf;\n}\n"
	if err := os.WriteFile(filepath.Join(dirs["etc"], "nginx.conf"), []byte(nginxConf), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := nginx.NewRenderer(root, root, dirs["accesslists"], dirs["snippets"])
	if err != nil {
		t.Fatal(err)
	}
	mgr := nginx.NewManager(dirs["available"], dirs["enabled"], r)
	return env{mgr: mgr, c: &Checker{
		Manager: mgr, ConfigDir: dirs["etc"], HTTPDir: dirs["conf.d"],
		SnippetsDir: dirs["snippets"], AccessListDir: dirs["accesslists"],
	}}
}

func (e env) addHost(t *testing.T, h nginx.Host, enable bool) {
	t.Helper()
	if err := e.mgr.WriteHost(h); err != nil {
		t.Fatal(err)
	}
	if enable {
		if err := e.mgr.Enable(h.ServerName); err != nil {
			t.Fatal(err)
		}
	}
}

func proxyHost(name string) nginx.Host {
	return nginx.Host{ServerName: name, Mode: nginx.ModeReverseProxy, Listen: 80,
		Locations: []nginx.Location{{Path: "/", Proxy: &nginx.ProxyConfig{
			UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}}}}
}

func run(t *testing.T, e env) []Finding {
	t.Helper()
	f, err := e.c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func has(findings []Finding, sev Severity, substr string) bool {
	for _, f := range findings {
		if f.Severity == sev && strings.Contains(f.Message, substr) {
			return true
		}
	}
	return false
}

func TestCleanConfigHasNoFindings(t *testing.T) {
	e := newEnv(t)
	e.addHost(t, proxyHost("example.com"), true)
	if f := run(t, e); len(f) != 0 {
		t.Errorf("expected no findings, got %+v", f)
	}
}

func TestDuplicateServerNames(t *testing.T) {
	e := newEnv(t)
	a := proxyHost("example.com")
	b := proxyHost("example.org")
	b.ServerAlias = []string{"EXAMPLE.com"}
	e.addHost(t, a, true)
	e.addHost(t, b, true)
	if f := run(t, e); !has(f, Error, "used by 2 enabled hosts") {
		t.Errorf("duplicate not reported: %+v", f)
	}

	// A disabled host does not conflict.
	if err := e.mgr.Disable("example.org"); err != nil {
		t.Fatal(err)
	}
	if f := run(t, e); has(f, Error, "used by 2 enabled hosts") {
		t.Errorf("disabled host reported as duplicate: %+v", f)
	}
}

func TestMissingReferencesAndUnusedFiles(t *testing.T) {
	e := newEnv(t)
	h := proxyHost("example.com")
	h.IPListName = "office"
	h.RateLimit = "api"
	h.Presets = []string{"static-cache"}
	h.Locations[0].AccessListName = "admins"
	e.addHost(t, h, true)
	os.WriteFile(filepath.Join(e.c.SnippetsDir, nginx.IPListFileName("unused")), []byte("deny all;\n"), 0o644)

	f := run(t, e)
	for _, want := range []string{"IP access list", "rate limit zone", "preset http-level file", "access list"} {
		if !has(f, Error, want+" ") {
			t.Errorf("missing %q not reported: %+v", want, f)
		}
	}
	if !has(f, Info, "not used by any GoNix host") {
		t.Errorf("unused list not reported: %+v", f)
	}
}

func TestIneffectiveSettings(t *testing.T) {
	e := newEnv(t)
	h := proxyHost("example.com")
	h.IPListName = "office"
	h.Presets = []string{"security-headers"}
	h.Locations = append(h.Locations,
		nginx.Location{Path: "/health", IsCustom: true, CustomConfig: "        return 200 'ok';"},
		nginx.Location{Path: "/files/", IsCustom: true, CustomConfig: "        add_header X-Test 1;"},
	)
	e.addHost(t, h, true)
	f := run(t, e)
	if !has(f, Warning, "/health uses return") || !has(f, Warning, "/files/ has its own add_header") {
		t.Errorf("ineffective settings not reported: %+v", f)
	}
}

func TestNginxConfMissingInclude(t *testing.T) {
	e := newEnv(t)
	os.WriteFile(filepath.Join(e.c.HTTPDir, "gonix-realip.conf"), []byte("real_ip_header X-Real-IP;\n"), 0o644)
	os.WriteFile(filepath.Join(e.c.HTTPDir, nginx.MaintenanceFileName("gone.example.com")), []byte("geo $x {}\n"), 0o644)
	os.WriteFile(filepath.Join(e.c.ConfigDir, "nginx.conf"), []byte("http {}\n"), 0o644)
	f := run(t, e)
	if !has(f, Error, "does not include") || !has(f, Info, "no longer in maintenance") {
		t.Errorf("findings = %+v", f)
	}
}

func TestCertificates(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	for _, tc := range []struct {
		host     string
		names    []string
		validFor time.Duration
		want     string
		sev      Severity
	}{
		{"expired.example.com", []string{"expired.example.com"}, -time.Hour, "expired on", Error},
		{"soon.example.com", []string{"soon.example.com"}, 5 * 24 * time.Hour, "expires in", Warning},
		{"wild.example.com", []string{"*.example.com"}, 90 * 24 * time.Hour, "", Warning},
		{"other.example.com", []string{"example.org"}, 90 * 24 * time.Hour, "does not list", Warning},
	} {
		cert, key := writeCert(t, filepath.Join(dir, tc.host), tc.names, tc.validFor)
		h := proxyHost(tc.host)
		h.ListenSSL = 443
		h.SSL = nginx.SSLConfig{Enabled: true, CertFile: cert, KeyFile: key, ForceHTTPS: true}
		e.addHost(t, h, true)
		f := run(t, e)
		if tc.want != "" && !has(f, tc.sev, tc.want) {
			t.Errorf("%s: %q not reported: %+v", tc.host, tc.want, f)
		}
		for _, x := range f {
			if x.Target == "wild.example.com" {
				t.Errorf("valid wildcard certificate reported: %+v", x)
			}
		}
	}

	h := proxyHost("plain.example.com")
	h.ListenSSL = 443
	h.SSL = nginx.SSLConfig{Enabled: true, CertFile: filepath.Join(dir, "none.pem"), KeyFile: filepath.Join(dir, "none.key")}
	e.addHost(t, h, true)
	f := run(t, e)
	if !has(f, Warning, "not redirected to HTTPS") || !has(f, Error, "is missing") {
		t.Errorf("findings = %+v", f)
	}
}

func TestUnreachableUpstream(t *testing.T) {
	e := newEnv(t)
	e.c.CheckUpstreams = true
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	h := proxyHost("example.com")
	h.Locations[0].Proxy.UpstreamPort = port
	e.addHost(t, h, true)
	if f := run(t, e); !has(f, Warning, "127.0.0.1:"+strconv.Itoa(port)+" is not accepting") {
		t.Errorf("down upstream not reported: %+v", f)
	}
}

func writeCert(t *testing.T, dir string, names []string, validFor time.Duration) (string, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(validFor),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	os.MkdirAll(dir, 0o755)
	cert, keyPath := filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
	return cert, keyPath
}

func TestUpstreamGroups(t *testing.T) {
	e := newEnv(t)
	e.c.CheckUpstreams = true
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := l.Addr().String()
	l.Close()
	group := "upstream gonix_app {\n    server " + dead + " max_fails=1;\n}\n"
	os.WriteFile(filepath.Join(e.c.HTTPDir, nginx.UpstreamFileName("app")), []byte(group), 0o644)
	os.WriteFile(filepath.Join(e.c.HTTPDir, nginx.UpstreamFileName("spare")), []byte(group), 0o644)

	h := proxyHost("example.com")
	h.Locations[0].Proxy.UpstreamGroup = "app"
	h.Locations = append(h.Locations, nginx.Location{Path: "/b/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamGroup: "gone", HTTPVersion: "1.1"}})
	e.addHost(t, h, true)
	f := run(t, e)
	if !has(f, Error, "upstream group") || !has(f, Info, "upstream group is not used") || !has(f, Warning, dead+" is not accepting") {
		t.Errorf("findings = %+v", f)
	}
}

func TestProxyCacheWithoutBuffering(t *testing.T) {
	e := newEnv(t)
	h := proxyHost("example.com")
	h.Presets = []string{"proxy-cache"}
	e.addHost(t, h, true)
	if f := run(t, e); !has(f, Warning, "buffering off") {
		t.Errorf("findings = %+v", f)
	}
}
