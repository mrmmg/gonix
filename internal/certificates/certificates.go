// Package certificates discovers and inspects TLS certificates stored under
// a centralized directory (see internal/config CertificatesConfig), using
// Go's standard crypto/x509 instead of shelling out to openssl.
package certificates

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// KeyType identifies the private key algorithm.
type KeyType string

const (
	KeyTypeUnknown KeyType = "unknown"
	KeyTypeRSA     KeyType = "RSA"
	KeyTypeECDSA   KeyType = "ECDSA"
)

// Status classifies a certificate's validity window.
type Status string

const (
	StatusValid        Status = "valid"
	StatusExpiringSoon Status = "expiring_soon"
	StatusExpired      Status = "expired"
	StatusInvalid      Status = "invalid"
)

// ExpiringSoonWindow is how far in advance a certificate is flagged as
// "expiring soon".
const ExpiringSoonWindow = 21 * 24 * time.Hour

// Certificate describes one discovered certificate directory.
type Certificate struct {
	Domain      string // directory name, typically the primary domain
	Dir         string
	CertPath    string
	KeyPath     string
	NotBefore   time.Time
	NotAfter    time.Time
	Subject     string
	Issuer      string
	SANs        []string
	KeyType     KeyType
	Fingerprint string // SHA-256 fingerprint of the leaf certificate, hex
	Status      Status
	ParseError  string // set when the PEM/certificate could not be parsed
	KeyMatches  bool   // whether the private key matches the certificate
}

// RemainingHuman returns a human readable description of the time left
// until expiration, e.g. "expires in approximately 20 days" or
// "expired 3 days ago".
func (c Certificate) RemainingHuman() string {
	d := time.Until(c.NotAfter)
	if d <= 0 {
		return fmt.Sprintf("expired approximately %s ago", humanDuration(-d))
	}
	return fmt.Sprintf("expires in approximately %s", humanDuration(d))
}

// ExpiresAtExact returns the expiration timestamp formatted as Y-m-d H:i:s.
func (c Certificate) ExpiresAtExact() string {
	return c.NotAfter.Format("2006-01-02 15:04:05")
}

func humanDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case days >= 2:
		return fmt.Sprintf("%d days", days)
	case days == 1:
		return "1 day"
	case d.Hours() >= 1:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return "less than an hour"
	}
}

// Scan walks certDir looking for subdirectories that contain both
// fullchain.pem and privkey.pem, and parses each one found.
func Scan(certDir string) ([]Certificate, error) {
	entries, err := os.ReadDir(certDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading certificates directory: %w", err)
	}

	var certs []Certificate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(certDir, e.Name())
		certPath := filepath.Join(dir, "fullchain.pem")
		keyPath := filepath.Join(dir, "privkey.pem")
		if !fileExists(certPath) || !fileExists(keyPath) {
			continue
		}
		certs = append(certs, Parse(e.Name(), dir, certPath, keyPath))
	}
	sort.Slice(certs, func(i, j int) bool { return certs[i].Domain < certs[j].Domain })
	return certs, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Parse loads and inspects a single certificate/key pair.
func Parse(domain, dir, certPath, keyPath string) Certificate {
	c := Certificate{Domain: domain, Dir: dir, CertPath: certPath, KeyPath: keyPath}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		c.ParseError = fmt.Sprintf("reading certificate: %v", err)
		c.Status = StatusInvalid
		return c
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		c.ParseError = "no PEM data found in certificate file"
		c.Status = StatusInvalid
		return c
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		c.ParseError = fmt.Sprintf("parsing certificate: %v", err)
		c.Status = StatusInvalid
		return c
	}

	c.NotBefore = leaf.NotBefore
	c.NotAfter = leaf.NotAfter
	c.Subject = leaf.Subject.String()
	c.Issuer = leaf.Issuer.String()
	c.SANs = leaf.DNSNames
	sum := sha256.Sum256(leaf.Raw)
	c.Fingerprint = fmt.Sprintf("%x", sum)

	switch leaf.PublicKey.(type) {
	case *rsa.PublicKey:
		c.KeyType = KeyTypeRSA
	case *ecdsa.PublicKey:
		c.KeyType = KeyTypeECDSA
	default:
		c.KeyType = KeyTypeUnknown
	}

	now := time.Now()
	switch {
	case now.After(leaf.NotAfter):
		c.Status = StatusExpired
	case leaf.NotAfter.Sub(now) <= ExpiringSoonWindow:
		c.Status = StatusExpiringSoon
	default:
		c.Status = StatusValid
	}

	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		c.KeyMatches = true
	}

	return c
}

// VerifyChain checks that the certificate at certPath can be verified using
// the intermediates bundled in the same file (a typical fullchain.pem
// contains leaf + intermediates). It does not check against the system root
// store since internal/private CAs are common for internal services.
func VerifyChain(certPath string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("reading certificate: %w", err)
	}

	var leaf *x509.Certificate
	intermediates := x509.NewCertPool()
	rest := data
	first := true
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("parsing certificate in chain: %w", err)
		}
		if first {
			leaf = cert
			first = false
			continue
		}
		intermediates.AddCert(cert)
	}
	if leaf == nil {
		return fmt.Errorf("no certificate found in %s", certPath)
	}

	_, err = leaf.Verify(x509.VerifyOptions{
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	return err
}
