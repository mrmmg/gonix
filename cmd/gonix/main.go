// Command GoNix is an interactive terminal UI for managing Nginx
// virtual hosts, TLS certificates, access lists and the Nginx service
// itself, without hiding the underlying, human-editable Nginx configuration.
package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/accesslist"
	"github.com/mrmmg/gonix/internal/acme"
	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/backup"
	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/errorpages"
	"github.com/mrmmg/gonix/internal/geoblock"
	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/maintenance"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/presets"
	"github.com/mrmmg/gonix/internal/ratelimit"
	"github.com/mrmmg/gonix/internal/realip"
	"github.com/mrmmg/gonix/internal/snippet"
	"github.com/mrmmg/gonix/internal/streams"
	"github.com/mrmmg/gonix/internal/system"
	"github.com/mrmmg/gonix/internal/tui"
	"github.com/mrmmg/gonix/internal/upstreams"
)

func main() {
	switch {
	case len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v"):
		fmt.Println("gonix version " + tui.Version)
		return
	case len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h"):
		printUsage()
		return
	case len(os.Args) > 2 && os.Args[1] == "acme-hook":
		// Hidden plumbing command: certbot's --manual-auth-hook/
		// --manual-cleanup-hook invoke "gonix acme-hook auth|cleanup" as a
		// subprocess (see internal/acme and internal/tui's wildcard
		// certificate wizard). Not meant to be run by hand.
		if err := acme.RunHook(context.Background(), os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "gonix acme-hook:", err)
			os.Exit(1)
		}
		return
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gonix:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`gonix - an interactive terminal UI for managing Nginx

Usage:
  sudo gonix              Launch the TUI
  gonix --version, -v     Print the installed version
  gonix --help, -h        Show this help message

Environment:
  GONIX_CONFIG   Path to an alternate configuration file
                 (default: ` + config.DefaultPath + `)
`)
}

func run() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this operation requires root privileges.\nPlease run gonix with sudo:\n\n    sudo gonix")
	}

	cfgPath := config.DefaultPath
	if v := os.Getenv("GONIX_CONFIG"); v != "" {
		cfgPath = v
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	renderer, err := nginx.NewRenderer(cfg.Logs.NginxDirectory, cfg.Logs.NginxDirectory, cfg.AccessLists.Directory, cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		return fmt.Errorf("initializing template renderer: %w", err)
	}
	// Version-dependent syntax (e.g. `http2 on;` on 1.25.1+); an unknown
	// build falls back to syntax every version accepts.
	if build, err := nginx.DetectBuild(context.Background(), cfg.Nginx.BinaryPath); err == nil {
		renderer.Build = build
	}
	manager := nginx.NewManager(cfg.Nginx.SitesAvailable, cfg.Nginx.SitesEnabled, renderer)
	validator := nginx.NewValidator(cfg.Nginx.BinaryPath)
	systemSvc := system.NewService(cfg.Nginx.BinaryPath)
	backups := backup.New(cfg.Backup.Directory, cfg.Backup.KeepCount)

	auditLogger, err := audit.NewLogger(cfg.Logs.AuditFile)
	if err != nil {
		return fmt.Errorf("initializing audit log: %w", err)
	}

	// Nginx workers, not the root master process, read htpasswd files, so
	// they must be owned by the worker group.
	if cfg.AccessLists.Group == "" {
		group, err := nginx.DetectWorkerGroup(context.Background(), cfg.Nginx.BinaryPath, cfg.Nginx.ConfigDir)
		if err != nil {
			return fmt.Errorf("%w; set access_lists.group in %s", err, cfgPath)
		}
		cfg.AccessLists.Group = group
	}
	accessLists, err := accesslist.NewStore(cfg.AccessLists.Directory, cfg.AccessLists.Group)
	if err != nil {
		return fmt.Errorf("initializing access list store: %w", err)
	}

	errorPagesStore, err := errorpages.NewStore(cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		return fmt.Errorf("initializing error pages store: %w", err)
	}

	hostService := &hosts.Service{
		Manager:  manager,
		Tester:   validator,
		Reloader: systemSvc,
		Backups:  backups,
		Audit:    auditLogger,
	}

	errorPagesService := &errorpages.Service{
		Store:    errorPagesStore,
		Manager:  manager,
		Tester:   validator,
		Reloader: systemSvc,
		Audit:    auditLogger,
	}

	// Standalone snippets (IP lists, real client IP, ...) share one safe
	// write -> nginx -t -> reload (or restore) workflow.
	snippetWriter := &snippet.Writer{Tester: validator, Reloader: systemSvc, Audit: auditLogger}

	ipListStore, err := iplist.NewStore(cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		return fmt.Errorf("initializing IP access list store: %w", err)
	}

	streamService := &streams.Service{
		Dir: cfg.Nginx.StreamsDir, MainIncludeDir: cfg.Nginx.MainIncludeDir, ConfigDir: cfg.Nginx.ConfigDir,
		SnippetsDir: cfg.ErrorPages.SnippetsDirectory, Writer: snippetWriter,
	}

	deps := tui.Deps{
		Config:      cfg,
		Manager:     manager,
		HostService: hostService,
		AccessLists: accessLists,
		ErrorPages:  errorPagesService,
		IPLists:     &iplist.Service{Store: ipListStore, Manager: manager, Writer: snippetWriter, OtherUsers: streamService.UsingIPList},
		RealIP:      &realip.Service{Dir: cfg.Nginx.HTTPSnippetsDir, Writer: snippetWriter},
		RateLimits: &ratelimit.Service{
			SnippetsDir: cfg.ErrorPages.SnippetsDirectory, HTTPDir: cfg.Nginx.HTTPSnippetsDir,
			Manager: manager, Writer: snippetWriter,
		},
		Upstreams: &upstreams.Service{Dir: cfg.Nginx.HTTPSnippetsDir, Manager: manager, Writer: snippetWriter},
		Streams:   streamService,
		GeoBlock: &geoblock.Service{
			HTTPDir: cfg.Nginx.HTTPSnippetsDir, SnippetsDir: cfg.ErrorPages.SnippetsDirectory,
			Database: cfg.GeoIP.Database, Manager: manager, Writer: snippetWriter,
		},
		Maintenance: &maintenance.Service{HTTPDir: cfg.Nginx.HTTPSnippetsDir, Hosts: hostService, Writer: snippetWriter},
		Presets:     &presets.Service{SnippetsDir: cfg.ErrorPages.SnippetsDirectory, HTTPDir: cfg.Nginx.HTTPSnippetsDir, CacheDir: cfg.Nginx.CacheDir, Writer: snippetWriter},
		SystemSvc:   systemSvc,
		Audit:       auditLogger,
		Backups:     backups,
	}

	p := tea.NewProgram(tui.NewApp(deps), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
