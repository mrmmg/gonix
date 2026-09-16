package acme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallCertificateCopiesAndChmods(t *testing.T) {
	liveDir := t.TempDir()
	certDir := t.TempDir()
	oldLive := letsEncryptLiveDir
	letsEncryptLiveDir = liveDir
	t.Cleanup(func() { letsEncryptLiveDir = oldLive })

	domainDir := filepath.Join(liveDir, "example.com")
	if err := os.MkdirAll(domainDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(domainDir, "fullchain.pem"), []byte("fullchain"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(domainDir, "privkey.pem"), []byte("privkey"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := InstallCertificate("example.com", certDir); err != nil {
		t.Fatalf("InstallCertificate: %v", err)
	}

	for _, name := range []string{"fullchain.pem", "privkey.pem"} {
		dst := filepath.Join(certDir, "example.com", name)
		info, err := os.Stat(dst)
		if err != nil {
			t.Fatalf("stat %s: %v", dst, err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("%s mode = %o, want 0644", name, info.Mode().Perm())
		}
	}
}
