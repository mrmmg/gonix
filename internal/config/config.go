// Package config centralizes all filesystem paths and tunables used across
// GoNix, loaded from a YAML file so nothing is hard-coded.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultPath is where GoNix looks for its configuration file.
const DefaultPath = "/etc/gonix/gonix.yaml"

// Config is the root configuration structure, mirroring the YAML file.
type Config struct {
	// Instance names this configuration when one server runs several
	// Nginx instances (see InstancesDir). GoNix's own files (backups,
	// history, audit log, access lists) then default to per-instance
	// locations, so instances never mix.
	Instance string `yaml:"instance,omitempty"`

	Nginx        NginxConfig        `yaml:"nginx"`
	Certificates CertificatesConfig `yaml:"certificates"`
	Logs         LogsConfig         `yaml:"logs"`
	Backup       BackupConfig       `yaml:"backup"`
	AccessLists  AccessListsConfig  `yaml:"access_lists"`
	ErrorPages   ErrorPagesConfig   `yaml:"error_pages"`
	GeoIP        GeoIPConfig        `yaml:"geoip"`
	History      HistoryConfig      `yaml:"history"`
}

type HistoryConfig struct {
	// Enabled commits the Nginx configuration to a Git repository after
	// every successful change (requires git).
	Enabled bool `yaml:"enabled"`
	// GitDir is that repository, kept outside the Nginx directory.
	GitDir string `yaml:"git_dir"`
}

type GeoIPConfig struct {
	// Database is the MaxMind GeoLite2/GeoIP2 Country (or City) database
	// used by country blocking, read by the third-party geoip2 module.
	Database string `yaml:"database"`
}

type NginxConfig struct {
	ConfigDir      string `yaml:"config_dir"`
	SitesAvailable string `yaml:"sites_available"`
	SitesEnabled   string `yaml:"sites_enabled"`
	BinaryPath     string `yaml:"binary_path"`
	// ConfigFile is passed to nginx -t with -c; empty uses the binary's
	// compiled-in default (normally <config_dir>/nginx.conf).
	ConfigFile string `yaml:"config_file"`
	// Service is the systemd unit reloaded after changes.
	Service string `yaml:"service"`
	// HTTPSnippetsDir holds http-level files GoNix generates (real client
	// IP, rate limit zones, ...). nginx.conf must include it inside the
	// http block, as Debian/Ubuntu/RHEL do with conf.d by default.
	HTTPSnippetsDir string `yaml:"http_snippets_dir"`
	// CacheDir is where the "proxy-cache" preset stores cached responses.
	CacheDir string `yaml:"cache_dir"`
	// StreamsDir holds GoNix's TCP/UDP stream proxies, included inside a
	// stream { } block.
	StreamsDir string `yaml:"streams_dir"`
	// MainIncludeDir is a directory nginx.conf includes at main (top)
	// level, where GoNix may place the stream { } block that includes
	// StreamsDir (Debian/Ubuntu: /etc/nginx/modules-enabled).
	MainIncludeDir string `yaml:"main_include_dir"`
}

type CertificatesConfig struct {
	Directory string `yaml:"directory"`
}

type LogsConfig struct {
	NginxDirectory string `yaml:"nginx_directory"`
	AuditFile      string `yaml:"audit_file"`
}

type BackupConfig struct {
	Directory string `yaml:"directory"`
	KeepCount int    `yaml:"keep_count"`
}

type AccessListsConfig struct {
	Directory string `yaml:"directory"`
	// Group owns the htpasswd files so Nginx worker processes can read
	// them. Empty means detect the Nginx worker group automatically.
	Group string `yaml:"group"`
}

type ErrorPagesConfig struct {
	// SnippetsDirectory is where GoNix writes the generated error page
	// snippets that hosts include.
	SnippetsDirectory string `yaml:"snippets_directory"`
	// PagesDirectory is the default directory holding the HTML pages
	// (<code>.html), offered when creating a new snippet.
	PagesDirectory string `yaml:"pages_directory"`
}

// Default returns the built-in configuration used when no configuration file
// is present on disk. It matches the standard Debian/RHEL Nginx layout.
func Default() *Config {
	return &Config{
		Nginx: NginxConfig{
			ConfigDir:       "/etc/nginx",
			SitesAvailable:  "/etc/nginx/sites-available",
			SitesEnabled:    "/etc/nginx/sites-enabled",
			BinaryPath:      "nginx",
			Service:         "nginx",
			HTTPSnippetsDir: "/etc/nginx/conf.d",
			CacheDir:        "/var/cache/nginx/gonix",
			StreamsDir:      "/etc/nginx/gonix-streams",
			MainIncludeDir:  "/etc/nginx/modules-enabled",
		},
		Certificates: CertificatesConfig{
			Directory: "/etc/nginx/certs",
		},
		Logs: LogsConfig{
			NginxDirectory: "/var/log/nginx",
			AuditFile:      "/var/log/gonix/audit.log",
		},
		Backup: BackupConfig{
			Directory: "/etc/gonix/backups",
			KeepCount: 20,
		},
		AccessLists: AccessListsConfig{
			Directory: "/etc/gonix/accesslists",
		},
		ErrorPages: ErrorPagesConfig{
			SnippetsDirectory: "/etc/nginx/snippets",
			PagesDirectory:    "/var/www/html/error_pages",
		},
		GeoIP: GeoIPConfig{
			Database: "/usr/share/GeoIP/GeoLite2-Country.mmdb",
		},
		History: HistoryConfig{
			Enabled: true,
			GitDir:  "/etc/gonix/history.git",
		},
	}
}

// Load reads the configuration file at path. If the file does not exist, the
// built-in defaults are returned without error so GoNix keeps
// working on a fresh install before the admin has customized anything.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	if cfg.Instance != "" {
		if !reInstance.MatchString(cfg.Instance) {
			return nil, fmt.Errorf("config file %s: instance name %q may only contain letters, digits, '-' and '_'", path, cfg.Instance)
		}
		applyInstanceDefaults(cfg)
	}
	return cfg, nil
}

// Save writes cfg to path as YAML, creating parent directories as needed.
func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config file %s: %w", path, err)
	}
	return nil
}

// InstancesDir holds one configuration file per additional Nginx instance,
// named <instance>.yaml and selected with --instance.
const InstancesDir = "/etc/gonix/instances"

var reInstance = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// InstancePath returns the configuration file of the named instance.
func InstancePath(name string) (string, error) {
	if !reInstance.MatchString(name) {
		return "", fmt.Errorf("instance name %q may only contain letters, digits, '-' and '_'", name)
	}
	return filepath.Join(InstancesDir, name+".yaml"), nil
}

// ListInstances returns the names of the configured instances.
func ListInstances(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".yaml"))
	}
	return names, nil
}

// applyInstanceDefaults moves GoNix's own files that were left at their
// defaults to per-instance locations.
func applyInstanceDefaults(cfg *Config) {
	def := Default()
	base := filepath.Join(filepath.Dir(DefaultPath), "instances", cfg.Instance)
	move := func(v *string, defaultValue, instanceValue string) {
		if *v == defaultValue {
			*v = instanceValue
		}
	}
	move(&cfg.Backup.Directory, def.Backup.Directory, filepath.Join(base, "backups"))
	move(&cfg.AccessLists.Directory, def.AccessLists.Directory, filepath.Join(base, "accesslists"))
	move(&cfg.History.GitDir, def.History.GitDir, filepath.Join(base, "history.git"))
	move(&cfg.Logs.AuditFile, def.Logs.AuditFile, filepath.Join(filepath.Dir(def.Logs.AuditFile), cfg.Instance+"-audit.log"))
}
