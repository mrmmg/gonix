package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestSaveMonitoringKeepsTheRestOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gonix.yaml")
	orig := "# my notes\nnginx:\n  sites_available: /srv/sites-available # custom\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Default().Monitoring
	m.Listen = "127.0.0.1:9999"
	m.Hosts = []string{"*.example.com"}
	if err := SaveMonitoring(path, m); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# my notes") || !strings.Contains(string(data), "# custom") {
		t.Errorf("comments lost:\n%s", data)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Monitoring.Listen != "127.0.0.1:9999" || cfg.Nginx.SitesAvailable != "/srv/sites-available" || cfg.Monitoring.Hosts[0] != "*.example.com" {
		t.Errorf("round trip: %+v", cfg)
	}
	// Saving again replaces the section rather than adding a second one.
	m.Listen = "127.0.0.1:9998"
	if err := SaveMonitoring(path, m); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Count(string(data), "monitoring:") != 1 {
		t.Errorf("monitoring section duplicated:\n%s", data)
	}
}

func TestSaveMonitoringCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "gonix.yaml")
	if err := SaveMonitoring(path, Default().Monitoring); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestMonitoringValidate(t *testing.T) {
	bad := []func(*MonitoringConfig){
		func(m *MonitoringConfig) { m.Listen = "9113" },
		func(m *MonitoringConfig) { m.Listen = "127.0.0.1:99999" },
		func(m *MonitoringConfig) { m.Collectors = []string{"nope"} },
		func(m *MonitoringConfig) { m.Hosts = []string{"[x"} },
		func(m *MonitoringConfig) { m.CacheTTL = -time.Second },
	}
	for i, f := range bad {
		m := Default().Monitoring
		f(&m)
		if m.Validate() == nil {
			t.Errorf("case %d: expected an error", i)
		}
	}
	if err := Default().Monitoring.Validate(); err != nil {
		t.Errorf("defaults invalid: %v", err)
	}
}

// Each instance runs its own exporter, so it must not default to the ports
// of the default instance or of another instance.
func TestInstancesGetTheirOwnExporterPorts(t *testing.T) {
	load := func(name string) *Config {
		path := filepath.Join(t.TempDir(), name+".yaml")
		os.WriteFile(path, []byte("instance: "+name+"\n"), 0o644)
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	a, b := load("staging"), load("prod")
	if a.Monitoring.Listen == Default().Monitoring.Listen || a.Monitoring.Listen == b.Monitoring.Listen {
		t.Errorf("listen: staging %s, prod %s", a.Monitoring.Listen, b.Monitoring.Listen)
	}
	if a.Monitoring.StubStatus.Listen == Default().Monitoring.StubStatus.Listen {
		t.Errorf("stub_status listen not per instance: %s", a.Monitoring.StubStatus.Listen)
	}
	if load("staging").Monitoring.Listen != a.Monitoring.Listen {
		t.Error("the port must be stable between runs")
	}
}

// The sample configuration shipped in configs/ must load, and say what the
// built-in defaults say, so copying it changes nothing by surprise.
func TestSampleConfigurationMatchesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "configs", "default.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Monitoring, Default().Monitoring; got.Listen != want.Listen || got.CacheTTL != want.CacheTTL || got.StubStatus != want.StubStatus {
		t.Errorf("monitoring in configs/default.yaml = %+v, defaults = %+v", got, want)
	}
}
