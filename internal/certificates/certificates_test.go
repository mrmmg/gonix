package certificates

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCert(t *testing.T, dir string, notAfter time.Time, ecdsaKey bool) {
	t.Helper()

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		DNSNames:     []string{"example.com", "www.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
	}

	var der []byte
	var keyPEM *pem.Block

	if ecdsaKey {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generating ecdsa key: %v", err)
		}
		der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatalf("creating certificate: %v", err)
		}
		keyBytes, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatalf("marshaling ecdsa key: %v", err)
		}
		keyPEM = &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}
	} else {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generating rsa key: %v", err)
		}
		der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatalf("creating certificate: %v", err)
		}
		keyPEM = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	certOut, err := os.Create(filepath.Join(dir, "fullchain.pem"))
	if err != nil {
		t.Fatalf("create fullchain.pem: %v", err)
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("encoding cert: %v", err)
	}

	keyOut, err := os.Create(filepath.Join(dir, "privkey.pem"))
	if err != nil {
		t.Fatalf("create privkey.pem: %v", err)
	}
	defer keyOut.Close()
	if err := pem.Encode(keyOut, keyPEM); err != nil {
		t.Fatalf("encoding key: %v", err)
	}
}

func TestParseValidRSACertificate(t *testing.T) {
	dir := t.TempDir()
	certDir := filepath.Join(dir, "example.com")
	writeCert(t, certDir, time.Now().Add(90*24*time.Hour), false)

	c := Parse("example.com", certDir, filepath.Join(certDir, "fullchain.pem"), filepath.Join(certDir, "privkey.pem"))
	if c.ParseError != "" {
		t.Fatalf("unexpected parse error: %s", c.ParseError)
	}
	if c.KeyType != KeyTypeRSA {
		t.Errorf("expected RSA key type, got %s", c.KeyType)
	}
	if c.Status != StatusValid {
		t.Errorf("expected valid status, got %s", c.Status)
	}
	if !c.KeyMatches {
		t.Error("expected key to match certificate")
	}
	if len(c.SANs) != 2 {
		t.Errorf("expected 2 SANs, got %v", c.SANs)
	}
}

func TestParseECDSACertificate(t *testing.T) {
	dir := t.TempDir()
	certDir := filepath.Join(dir, "example.com")
	writeCert(t, certDir, time.Now().Add(90*24*time.Hour), true)

	c := Parse("example.com", certDir, filepath.Join(certDir, "fullchain.pem"), filepath.Join(certDir, "privkey.pem"))
	if c.KeyType != KeyTypeECDSA {
		t.Errorf("expected ECDSA key type, got %s", c.KeyType)
	}
}

func TestExpiredCertificateStatus(t *testing.T) {
	dir := t.TempDir()
	certDir := filepath.Join(dir, "expired.com")
	writeCert(t, certDir, time.Now().Add(-24*time.Hour), false)

	c := Parse("expired.com", certDir, filepath.Join(certDir, "fullchain.pem"), filepath.Join(certDir, "privkey.pem"))
	if c.Status != StatusExpired {
		t.Errorf("expected expired status, got %s", c.Status)
	}
}

func TestExpiringSoonStatus(t *testing.T) {
	dir := t.TempDir()
	certDir := filepath.Join(dir, "soon.com")
	writeCert(t, certDir, time.Now().Add(5*24*time.Hour), false)

	c := Parse("soon.com", certDir, filepath.Join(certDir, "fullchain.pem"), filepath.Join(certDir, "privkey.pem"))
	if c.Status != StatusExpiringSoon {
		t.Errorf("expected expiring_soon status, got %s", c.Status)
	}
}

func TestDueForRenewal(t *testing.T) {
	dir := t.TempDir()

	freshDir := filepath.Join(dir, "fresh.com")
	writeCert(t, freshDir, time.Now().Add(80*24*time.Hour), false)
	fresh := Parse("fresh.com", freshDir, filepath.Join(freshDir, "fullchain.pem"), filepath.Join(freshDir, "privkey.pem"))
	if fresh.DueForRenewal() {
		t.Error("a certificate with 80 days left should not be due for renewal")
	}

	dueDir := filepath.Join(dir, "due.com")
	writeCert(t, dueDir, time.Now().Add(10*24*time.Hour), false)
	due := Parse("due.com", dueDir, filepath.Join(dueDir, "fullchain.pem"), filepath.Join(dueDir, "privkey.pem"))
	if !due.DueForRenewal() {
		t.Error("a certificate with 10 days left should be due for renewal")
	}
}

func TestScanOnlyReturnsCompletePairs(t *testing.T) {
	dir := t.TempDir()
	writeCert(t, filepath.Join(dir, "complete.com"), time.Now().Add(90*24*time.Hour), false)

	// A directory with only a certificate, no key, should be ignored.
	incomplete := filepath.Join(dir, "incomplete.com")
	if err := os.MkdirAll(incomplete, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incomplete, "fullchain.pem"), []byte("not a real cert"), 0o644); err != nil {
		t.Fatal(err)
	}

	certs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(certs))
	}
	if certs[0].Domain != "complete.com" {
		t.Errorf("expected complete.com, got %s", certs[0].Domain)
	}
}

func TestParseInvalidPEM(t *testing.T) {
	dir := t.TempDir()
	certDir := filepath.Join(dir, "broken.com")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := Parse("broken.com", certDir, filepath.Join(certDir, "fullchain.pem"), filepath.Join(certDir, "privkey.pem"))
	if c.ParseError == "" {
		t.Fatal("expected parse error for invalid PEM")
	}
	if c.Status != StatusInvalid {
		t.Errorf("expected invalid status, got %s", c.Status)
	}
}
