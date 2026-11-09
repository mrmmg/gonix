// Package config centralizes all filesystem paths and tunables used across
// GoNix, loaded from a YAML file so nothing is hard-coded.
package config

import (
	"fmt"
	"hash/crc32"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

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
	Monitoring   MonitoringConfig   `yaml:"monitoring"`
	Update       UpdateConfig       `yaml:"update"`
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
	// HTTPPort and HTTPSPort are the ports new hosts listen on.
	HTTPPort  int `yaml:"http_port"`
	HTTPSPort int `yaml:"https_port"`
}

type CertificatesConfig struct {
	Directory string `yaml:"directory"`
	// LetsEncryptDir is certbot's configuration directory. Its live
	// certificates are listed next to GoNix's own, and renewed with certbot
	// when it is installed.
	LetsEncryptDir string `yaml:"letsencrypt_dir"`
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
			HTTPPort:        80,
			HTTPSPort:       443,
		},
		Certificates: CertificatesConfig{
			Directory:      "/etc/nginx/certs",
			LetsEncryptDir: "/etc/letsencrypt",
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
		Update: UpdateConfig{Check: true, RestartExporter: true},
		Monitoring: MonitoringConfig{
			Listen:     "127.0.0.1:9113",
			CacheTTL:   30 * time.Second,
			StubStatus: StubStatusConfig{Listen: "127.0.0.1:9114"},
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
	if err := cfg.Monitoring.Validate(); err != nil {
		return nil, fmt.Errorf("config file %s: %w", path, err)
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
	// Every instance runs its own exporter, so each needs its own ports.
	n := crc32.ChecksumIEEE([]byte(cfg.Instance)) % 400
	move(&cfg.Monitoring.Listen, def.Monitoring.Listen, "127.0.0.1:"+strconv.Itoa(9200+int(n)))
	move(&cfg.Monitoring.StubStatus.Listen, def.Monitoring.StubStatus.Listen, "127.0.0.1:"+strconv.Itoa(9600+int(n)))
	move(&cfg.Logs.AuditFile, def.Logs.AuditFile, filepath.Join(filepath.Dir(def.Logs.AuditFile), cfg.Instance+"-audit.log"))
}

// Collectors are the metric groups the exporter can serve.
var Collectors = []string{"gonix", "certificates", "upstreams", "nginx", "traffic", "logfiles"}

// MonitoringConfig configures the Prometheus exporter (gonix exporter).
type MonitoringConfig struct {
	// Listen is the address the exporter serves /metrics on. It stays on
	// loopback unless changed on purpose.
	Listen string `yaml:"listen"`
	// Hosts limits per-host metrics to hosts matching these patterns
	// (shell-style, e.g. "*.example.com"); empty means every host.
	Hosts []string `yaml:"hosts"`
	// ExcludeHosts removes hosts matching these patterns.
	ExcludeHosts []string `yaml:"exclude_hosts"`
	// Collectors lists the enabled metric groups (see Collectors); empty
	// means all of them.
	Collectors []string `yaml:"collectors"`
	// CacheTTL is how long the results of slow collectors (certificates,
	// upstream probes, nginx -t) are reused between scrapes.
	CacheTTL time.Duration `yaml:"cache_ttl"`
	// StubStatus adds Nginx's connection counters (needs the
	// http_stub_status_module).
	StubStatus StubStatusConfig `yaml:"stub_status"`
}

// UpdateConfig controls how GoNix learns about, and applies, new releases.
// The source is always this project's GitHub Releases.
type UpdateConfig struct {
	// Check makes GoNix ask GitHub, at most once a day, whether a newer
	// release exists and say so in the main menu. Turn it off on servers
	// that must not contact the internet.
	Check bool `yaml:"check"`
	// RestartExporter restarts the running Prometheus exporter (of every
	// instance) after `gonix update`, so it runs the new version.
	RestartExporter bool `yaml:"restart_exporter"`
}

// StubStatusConfig is a local-only Nginx server exposing stub_status.
type StubStatusConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

// ValidListen reports whether addr is a host:port with a usable port.
func ValidListen(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("address %q must look like 127.0.0.1:9113", addr)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("address %q has an invalid port", addr)
	}
	return nil
}

// Validate checks the monitoring settings.
func (m MonitoringConfig) Validate() error {
	if err := ValidListen(m.Listen); err != nil {
		return fmt.Errorf("monitoring.listen: %w", err)
	}
	if err := ValidListen(m.StubStatus.Listen); err != nil {
		return fmt.Errorf("monitoring.stub_status.listen: %w", err)
	}
	if m.CacheTTL < 0 {
		return fmt.Errorf("monitoring.cache_ttl must not be negative")
	}
	for _, c := range m.Collectors {
		if !slices.Contains(Collectors, c) {
			return fmt.Errorf("monitoring.collectors: unknown collector %q (use %s)", c, strings.Join(Collectors, ", "))
		}
	}
	for _, p := range append(slices.Clone(m.Hosts), m.ExcludeHosts...) {
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("monitoring: bad host pattern %q", p)
		}
	}
	return nil
}

// SaveMonitoring replaces only the monitoring section of the file at path,
// leaving the rest of it (and its comments) as it was.
func SaveMonitoring(path string, m MonitoringConfig) error { return SaveSection(path, "monitoring", m) }

// SaveUpdate replaces only the update section of the file at path.
func SaveUpdate(path string, u UpdateConfig) error { return SaveSection(path, "update", u) }

// SaveSection replaces the top-level section key of the YAML file at path
// with v, leaving the rest of the file (and its comments) as it was.
func SaveSection(path, key string, m any) error {
	var doc yaml.Node
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing config file %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("config file %s is not a YAML mapping", path)
	}
	var node yaml.Node
	if err := node.Encode(m); err != nil {
		return err
	}
	replaced := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			root.Content[i+1], replaced = &node, true
		}
	}
	if !replaced {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &node)
	}
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
