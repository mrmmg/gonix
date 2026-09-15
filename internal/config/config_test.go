package config

import (
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
	path := filepath.Join(t.TempDir(), "nginx-manager.yaml")
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
