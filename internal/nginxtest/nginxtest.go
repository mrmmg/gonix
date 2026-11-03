// Package nginxtest lets tests check generated configuration against a real
// `nginx -t`, using a throwaway prefix so neither root privileges nor the
// system configuration are involved. Tests using it are skipped when no
// nginx binary is installed.
package nginxtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Prefix is a throwaway nginx prefix: an nginx.conf whose http block
// includes everything in HTTPDir (http-level snippets) and SitesDir
// (server blocks).
type Prefix struct {
	Dir      string
	HTTPDir  string
	SitesDir string
	binary   string
}

// New creates a Prefix, skipping the test when nginx is not installed.
func New(t testing.TB) *Prefix {
	t.Helper()
	bin, err := exec.LookPath("nginx")
	if err != nil {
		t.Skip("nginx binary not installed; skipping real nginx -t check")
	}
	dir := t.TempDir()
	p := &Prefix{
		Dir:      dir,
		HTTPDir:  filepath.Join(dir, "conf.d"),
		SitesDir: filepath.Join(dir, "sites"),
		binary:   bin,
	}
	for _, d := range []string{p.HTTPDir, p.SitesDir, filepath.Join(dir, "logs")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("creating %s: %v", d, err)
		}
	}
	conf := "error_log " + filepath.Join(dir, "logs", "error.log") + ";\n" +
		"pid " + filepath.Join(dir, "nginx.pid") + ";\n" +
		"events {}\n" +
		"http {\n" +
		"    access_log off;\n" +
		"    client_body_temp_path " + filepath.Join(dir, "tmp-body") + ";\n" +
		"    proxy_temp_path " + filepath.Join(dir, "tmp-proxy") + ";\n" +
		"    fastcgi_temp_path " + filepath.Join(dir, "tmp-fastcgi") + ";\n" +
		"    uwsgi_temp_path " + filepath.Join(dir, "tmp-uwsgi") + ";\n" +
		"    scgi_temp_path " + filepath.Join(dir, "tmp-scgi") + ";\n" +
		"    include " + filepath.Join(p.HTTPDir, "*.conf") + ";\n" +
		"    include " + filepath.Join(p.SitesDir, "*") + ";\n" +
		"}\n"
	p.write(t, filepath.Join(dir, "nginx.conf"), conf)
	return p
}

// AddHTTP writes an http-level snippet (e.g. a limit_req_zone file).
func (p *Prefix) AddHTTP(t testing.TB, name, content string) {
	t.Helper()
	p.write(t, filepath.Join(p.HTTPDir, name), content)
}

// AddSite writes a server block file. `nginx -t` binds every listen
// address, so privileged ports (80, 443, ...) are shifted up by 20000 to
// let tests run without root.
func (p *Prefix) AddSite(t testing.TB, name, content string) {
	t.Helper()
	p.write(t, filepath.Join(p.SitesDir, name), unprivilegedPorts(content))
}

var reListenPort = regexp.MustCompile(`(?m)^(\s*listen\s+(?:\[::\]:)?)(\d+)`)

func unprivilegedPorts(content string) string {
	return reListenPort.ReplaceAllStringFunc(content, func(m string) string {
		sub := reListenPort.FindStringSubmatch(m)
		port, _ := strconv.Atoi(sub[2])
		if port < 1024 {
			port += 20000
		}
		return sub[1] + strconv.Itoa(port)
	})
}

// Test runs `nginx -t` against the prefix and fails the test, printing the
// nginx output, when the configuration is rejected.
func (p *Prefix) Test(t testing.TB) {
	t.Helper()
	if out, ok := p.Run(t); !ok {
		t.Fatalf("nginx -t rejected the configuration:\n%s", out)
	}
}

// Run runs `nginx -t` and returns its output and whether it succeeded.
func (p *Prefix) Run(t testing.TB) (string, bool) {
	t.Helper()
	cmd := exec.Command(p.binary, "-t", "-p", p.Dir, "-c", filepath.Join(p.Dir, "nginx.conf"))
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// SelfSignedCert writes a throwaway ECDSA certificate/key pair for
// commonName into the prefix and returns their paths.
func (p *Prefix) SelfSignedCert(t testing.TB, commonName string) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     []string{commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling key: %v", err)
	}
	dir := filepath.Join(p.Dir, "certs", strings.ReplaceAll(commonName, "*", "_"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	certFile = filepath.Join(dir, "fullchain.pem")
	keyFile = filepath.Join(dir, "privkey.pem")
	p.write(t, certFile, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
	p.write(t, keyFile, string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})))
	return certFile, keyFile
}

func (p *Prefix) write(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
