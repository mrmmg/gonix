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

func TestInstanceDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "staging.yaml")
	content := "instance: staging\nnginx:\n  config_dir: /etc/nginx-staging\n  config_file: /etc/nginx-staging/nginx.conf\n  service: nginx-staging\nbackup:\n  directory: /srv/backups\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backup.Directory != "/srv/backups" {
		t.Errorf("an explicit path must be kept, got %s", cfg.Backup.Directory)
	}
	for got, want := range map[string]string{
		cfg.History.GitDir:        "/etc/gonix/instances/staging/history.git",
		cfg.AccessLists.Directory: "/etc/gonix/instances/staging/accesslists",
		cfg.Logs.AuditFile:        "/var/log/gonix/staging-audit.log",
		cfg.Nginx.Service:         "nginx-staging",
	} {
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
	os.WriteFile(filepath.Join(dir, "prod.yaml"), []byte("instance: prod\n"), 0o644)
	if names, _ := ListInstances(dir); len(names) != 2 || names[0] != "prod" || names[1] != "staging" {
		t.Errorf("ListInstances = %v", names)
	}
	if _, err := InstancePath("../x"); err == nil {
		t.Error("expected an invalid instance name to be rejected")
	}
	os.WriteFile(path, []byte("instance: ../bad\n"), 0o644)
	if _, err := Load(path); err == nil {
		t.Error("expected an invalid instance name in the file to be rejected")
	}

	// The main configuration (no instance) keeps the plain defaults.
	if cfg := Default(); cfg.Nginx.Service != "nginx" || cfg.History.GitDir != "/etc/gonix/history.git" {
		t.Errorf("defaults changed: %+v", cfg.Nginx)
	}
}
