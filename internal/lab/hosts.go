package lab

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/mrmmg/gonix/internal/hosts"
)

// Scenario is a kind of hand-written test host.
type Scenario struct {
	ID            string
	Title         string
	DefaultDomain string
	// Adopt says what `gonix adopt` makes of it.
	Adopt string
	// certbot hosts get an imitation certbot lineage, plus a copy of the
	// certificate in GoNix's directory to switch to.
	certbot bool
	// www hosts get a document root with an index page.
	www bool
	// fixedFile names the file regardless of the domain.
	fixedFile string
}

// Scenarios are the test hosts the lab can create, in menu order.
var Scenarios = []Scenario{
	{ID: "certbot-proxy", Title: "Reverse proxy after certbot --nginx", DefaultDomain: "app.lab.test", certbot: true,
		Adopt: "adopted; certificate renewed by certbot, separate HTTP block, /legacy/ kept as custom"},
	{ID: "proxy", Title: "Plain reverse proxy over HTTP", DefaultDomain: "api.lab.test",
		Adopt: "adopted; client_max_body_size kept as written"},
	{ID: "static", Title: "Static site with try_files (tabs)", DefaultDomain: "static.lab.test", www: true,
		Adopt: "adopted as static; index and try_files kept, /assets/ custom"},
	{ID: "redirect", Title: "Whole-domain redirect", DefaultDomain: "old.lab.test",
		Adopt: "adopted as a redirect host"},
	{ID: "wildcard", Title: "Wildcard name after the main one", DefaultDomain: "wild.lab.test",
		Adopt: "adopted; the wildcard name keeps working when aliases change"},
	{ID: "custom", Title: "PHP, named and WebSocket locations", DefaultDomain: "php.lab.test", www: true,
		Adopt: "adopted; /, @backend and the PHP location custom, /api/ a proxy with WebSocket"},
	{ID: "multi", Title: "Two sites in one file", DefaultDomain: "multi.lab.test",
		Adopt: "refused: two server blocks"},
	{ID: "default", Title: "Catch-all default server", DefaultDomain: "", fixedFile: "default",
		Adopt: "refused: server_name _ is not a domain"},
}

// ScenarioByID finds a scenario.
func ScenarioByID(id string) (Scenario, bool) {
	for _, s := range Scenarios {
		if s.ID == id {
			return s, true
		}
	}
	return Scenario{}, false
}

// NeedsDomain reports whether the scenario uses a domain name.
func (s Scenario) NeedsDomain() bool { return s.fixedFile == "" }

func (s Scenario) fileName(domain string) string {
	switch {
	case s.fixedFile != "":
		return s.fixedFile
	case s.certbot || s.ID == "multi":
		return domain + ".conf" // as certbot's tutorials and multi-site setups name them
	}
	return domain
}

// AddHost writes a test host of the given scenario, enables it, checks the
// lab's configuration with nginx -t and reloads the lab. A host that fails
// the check is removed again.
func (l *Lab) AddHost(ctx context.Context, st *State, scenario, domain string) (Host, error) {
	if err := l.checkRoot(); err != nil {
		return Host{}, err
	}
	sc, ok := ScenarioByID(scenario)
	if !ok {
		return Host{}, fmt.Errorf("unknown scenario %q", scenario)
	}
	if sc.NeedsDomain() {
		if err := hosts.ValidateDomain(domain); err != nil {
			return Host{}, err
		}
	} else {
		domain = ""
	}
	file := sc.fileName(domain)
	available := filepath.Join(st.Dir, "sites-available", file)
	enabled := filepath.Join(st.Dir, "sites-enabled", file)
	if _, err := os.Lstat(available); err == nil {
		return Host{}, fmt.Errorf("%s already exists", available)
	}

	var created []string // undone if the host fails nginx -t
	undo := func() {
		for i := len(created) - 1; i >= 0; i-- {
			os.RemoveAll(created[i])
		}
	}
	data := map[string]any{
		"Dir": st.Dir, "Domain": domain, "HTTPPort": st.HTTPPort, "HTTPSPort": st.HTTPSPort,
		"HTTPSBase": httpsBase(st.HTTPSPort), "BackendA": st.BackendA, "BackendB": st.BackendB,
		"Target": hostPort("static.lab.test", st.HTTPPort, 80), "WWW": filepath.Join(st.Dir, "www", domain),
		"LiveDir": filepath.Join(st.Dir, "letsencrypt", "live", domain),
	}
	if sc.www {
		root := filepath.Join(st.Dir, "www", domain)
		if err := writeFile(filepath.Join(root, "index.html"), "<h1>"+domain+"</h1>\n", 0o644); err != nil {
			return Host{}, err
		}
		created = append(created, root)
	}
	if sc.certbot {
		paths, err := l.certbotLineage(st, domain)
		created = append(created, paths...)
		if err != nil {
			undo()
			return Host{}, err
		}
	}
	if err := writeTemplate(available, "sites/"+sc.ID+".tmpl", data); err != nil {
		undo()
		return Host{}, err
	}
	created = append(created, available)
	if err := os.Symlink(available, enabled); err != nil {
		undo()
		return Host{}, err
	}
	created = append(created, enabled)

	if out, err := l.TestConfig(ctx, filepath.Join(st.Dir, "nginx.conf")); err != nil {
		undo()
		return Host{}, fmt.Errorf("nginx -t failed, so the host was removed again: %v\n%s", err, out)
	}
	if l.Active(ctx) {
		if err := l.systemctl(ctx, "reload", Unit); err != nil {
			return Host{}, err
		}
	}
	h := Host{Scenario: sc.ID, Domain: domain, File: file}
	st.Hosts = append(st.Hosts, h)
	return h, st.save()
}

func hostPort(host string, port, standard int) string {
	if port == standard {
		return host
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func httpsBase(port int) string {
	if port == 443 {
		return "https://$host"
	}
	return fmt.Sprintf("https://$host:%d", port)
}

// certbotLineage lays out a certificate the way certbot does (live/,
// renewal/<name>.conf), self-signed, and puts a copy in GoNix's
// certificates directory to switch to. It returns what it created.
func (l *Lab) certbotLineage(st *State, domain string) ([]string, error) {
	names := []string{domain, "www." + domain, "api." + domain}
	live := filepath.Join(st.Dir, "letsencrypt", "live", domain)
	renewal := filepath.Join(st.Dir, "letsencrypt", "renewal", domain+".conf")
	gonixCopy := filepath.Join(st.Dir, "certs", domain)
	for _, p := range []string{live, renewal, gonixCopy} {
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s already exists", p)
		}
	}
	var created []string
	for _, dir := range []string{live, gonixCopy} {
		created = append(created, dir)
		if err := selfSigned(dir, names); err != nil {
			return created, err
		}
	}
	created = append(created, renewal)
	return created, writeTemplate(renewal, "renewal.conf.tmpl", map[string]any{"Dir": st.Dir, "Domain": domain})
}

// selfSigned writes a self-signed ECDSA certificate for names into dir as
// certbot names its files.
func selfSigned(dir string, names []string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: names[0], Organization: []string{"GoNix lab"}},
		DNSNames:     names,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	cert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	for name, content := range map[string]string{"cert.pem": cert, "chain.pem": cert, "fullchain.pem": cert} {
		if err := writeFile(filepath.Join(dir, name), content, 0o644); err != nil {
			return err
		}
	}
	return writeFile(filepath.Join(dir, "privkey.pem"), string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})), 0o600)
}
