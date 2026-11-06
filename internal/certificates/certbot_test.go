package certificates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCertbotLineage(t *testing.T) {
	le := t.TempDir()
	live := filepath.Join(le, "live", "example.com")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(le, "renewal"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := "# renew_before_expiry = 30 days\nversion = 2.9.0\narchive_dir = /x\n\n[renewalparams]\naccount = abc\n" +
		"authenticator = nginx\ninstaller = nginx\nkey_type = ecdsa\n\n[[webroot_map]]\nexample.com = /var/www\n"
	if err := os.WriteFile(filepath.Join(le, "renewal", "example.com.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	l, ok := CertbotLineage(filepath.Join(live, "fullchain.pem"))
	if !ok {
		t.Fatal("lineage not found")
	}
	if l.Name != "example.com" || l.Dir != le || l.Authenticator != "nginx" || l.Installer != "nginx" || l.KeyType != "ecdsa" {
		t.Errorf("lineage = %+v", l)
	}
	if len(l.Notes()) != 3 {
		t.Errorf("notes = %q", l.Notes())
	}
	for _, p := range []string{"/etc/nginx/certs/example.com/fullchain.pem", filepath.Join(le, "live", "other.com", "fullchain.pem"), "fullchain.pem"} {
		if _, ok := CertbotLineage(p); ok {
			t.Errorf("%s is not a certbot lineage", p)
		}
	}
	if certs := ScanCertbot(filepath.Join(le, "missing")); certs != nil {
		t.Errorf("missing dir = %v", certs)
	}
}
