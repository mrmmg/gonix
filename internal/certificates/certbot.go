package certificates

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DefaultLetsEncryptDir is certbot's default configuration directory.
const DefaultLetsEncryptDir = "/etc/letsencrypt"

// Lineage describes a certificate certbot manages: certbot keeps it in
// <dir>/live/<name>/ and renews it on its own schedule (systemd timer or
// cron), as configured in <dir>/renewal/<name>.conf.
type Lineage struct {
	Name        string // certbot's --cert-name
	Dir         string // certbot's configuration directory, e.g. /etc/letsencrypt
	RenewalFile string
	// From the [renewalparams] section of the renewal file; empty when it
	// could not be read.
	Authenticator string // e.g. nginx, webroot, standalone, dns-cloudflare, manual
	Installer     string // e.g. nginx, or empty / "None"
	KeyType       string
}

// CertbotLineage reports whether certPath is a certificate certbot keeps in
// its live directory (<dir>/live/<name>/<file>, with a matching
// <dir>/renewal/<name>.conf), and reads how certbot renews it. It works
// for any certbot configuration directory, not only the default one.
func CertbotLineage(certPath string) (Lineage, bool) {
	certPath = filepath.Clean(certPath)
	nameDir := filepath.Dir(certPath)
	live := filepath.Dir(nameDir)
	if filepath.Base(live) != "live" {
		return Lineage{}, false
	}
	l := Lineage{Name: filepath.Base(nameDir), Dir: filepath.Dir(live)}
	l.RenewalFile = filepath.Join(l.Dir, "renewal", l.Name+".conf")
	params, err := readRenewal(l.RenewalFile)
	if err != nil {
		return Lineage{}, false
	}
	l.Authenticator, l.Installer, l.KeyType = params["authenticator"], params["installer"], params["key_type"]
	if l.Installer == "None" {
		l.Installer = ""
	}
	return l, true
}

// readRenewal reads the [renewalparams] section of a certbot renewal file
// (a flat "key = value" INI file).
func readRenewal(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	params := map[string]string{}
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "["):
			section = strings.Trim(line, "[]")
		case section == "renewalparams":
			if k, v, ok := strings.Cut(line, "="); ok {
				params[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	return params, sc.Err()
}

// ScanCertbot lists the certificates in certbot's live directory under
// leDir, each tagged with Source "certbot". A missing or unreadable
// directory (certbot keeps it root-only) yields no certificates.
func ScanCertbot(leDir string) []Certificate {
	certs, err := Scan(filepath.Join(leDir, "live"))
	if err != nil {
		return nil
	}
	for i := range certs {
		certs[i].Source = SourceCertbot
	}
	return certs
}

// Notes describes, in a sentence or two each, what managing a host whose
// certificate certbot renews means for that renewal.
func (l Lineage) Notes() []string {
	notes := []string{"certbot keeps renewing " + l.Name + " (" + l.RenewalFile + "); the file now uses the renewed certificate automatically"}
	switch l.Authenticator {
	case "nginx":
		notes = append(notes, "certbot renews it through its nginx plugin (HTTP-01), which needs a plain-HTTP server block for the domain: keep the host listening on port 80")
	case "webroot":
		notes = append(notes, "certbot renews it through a webroot (HTTP-01): keep /.well-known/acme-challenge/ served from it")
	case "standalone":
		notes = append(notes, "certbot renews it in standalone mode, which needs port 80 free during renewal (usually via pre/post hooks that stop Nginx)")
	}
	if l.Installer == "nginx" {
		notes = append(notes, "certbot's nginx installer edits server blocks when run as `certbot --nginx` or `certbot install`; renewals only reload Nginx")
	}
	return notes
}
