package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReturnsDefaultsWhenMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Nginx.SitesAvailable != "/etc/nginx/sites-available" {
		t.Errorf("expected default sites-available path, got %s", cfg.Nginx.SitesAvailable)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gonix.yaml")
	cfg := Default()
	cfg.Nginx.SitesAvailable = "/custom/sites-available"

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Nginx.SitesAvailable != "/custom/sites-available" {
		t.Errorf("expected round-tripped custom path, got %s", loaded.Nginx.SitesAvailable)
	}
}

// An existing gonix.yaml written before http_snippets_dir existed must keep
// loading, with the new key falling back to its default.
func TestLoadOldConfigKeepsNewDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gonix.yaml")
	old := "nginx:\n  config_dir: /etc/nginx\n  sites_available: /srv/sites-available\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Nginx.SitesAvailable != "/srv/sites-available" {
		t.Errorf("SitesAvailable = %s", cfg.Nginx.SitesAvailable)
	}
	if cfg.Nginx.HTTPSnippetsDir != "/etc/nginx/conf.d" {
		t.Errorf("HTTPSnippetsDir = %q, want default /etc/nginx/conf.d", cfg.Nginx.HTTPSnippetsDir)
	}
}
