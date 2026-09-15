// Package config centralizes all filesystem paths and tunables used across
// nginx-manager, loaded from a YAML file so nothing is hard-coded.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultPath is where nginx-manager looks for its configuration file.
const DefaultPath = "/etc/nginx-manager/nginx-manager.yaml"

// Config is the root configuration structure, mirroring the YAML file.
type Config struct {
	Nginx        NginxConfig        `yaml:"nginx"`
	Certificates CertificatesConfig `yaml:"certificates"`
	Logs         LogsConfig         `yaml:"logs"`
	Backup       BackupConfig       `yaml:"backup"`
	AccessLists  AccessListsConfig  `yaml:"access_lists"`
}

type NginxConfig struct {
	ConfigDir      string `yaml:"config_dir"`
	SitesAvailable string `yaml:"sites_available"`
	SitesEnabled   string `yaml:"sites_enabled"`
	BinaryPath     string `yaml:"binary_path"`
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
}

// Default returns the built-in configuration used when no configuration file
// is present on disk. It matches the standard Debian/RHEL Nginx layout.
func Default() *Config {
	return &Config{
		Nginx: NginxConfig{
			ConfigDir:      "/etc/nginx",
			SitesAvailable: "/etc/nginx/sites-available",
			SitesEnabled:   "/etc/nginx/sites-enabled",
			BinaryPath:     "nginx",
		},
		Certificates: CertificatesConfig{
			Directory: "/etc/nginx/certs",
		},
		Logs: LogsConfig{
			NginxDirectory: "/var/log/nginx",
			AuditFile:      "/var/log/nginx-manager/audit.log",
		},
		Backup: BackupConfig{
			Directory: "/etc/nginx-manager/backups",
			KeepCount: 20,
		},
		AccessLists: AccessListsConfig{
			Directory: "/etc/nginx-manager/accesslists",
		},
	}
}

// Load reads the configuration file at path. If the file does not exist, the
// built-in defaults are returned without error so nginx-manager keeps
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
