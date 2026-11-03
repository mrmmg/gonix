// Package app wires every GoNix service together from the configuration.
// The TUI and the command line both work on the same Services, so they
// share one implementation of every change and its safety checks.
package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mrmmg/gonix/internal/accesslist"
	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/backup"
	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/errorpages"
	"github.com/mrmmg/gonix/internal/geoblock"
	"github.com/mrmmg/gonix/internal/history"
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
	"github.com/mrmmg/gonix/internal/upstreams"
)

// Services bundles every backend dependency. Front ends must only interact
// with Nginx and the filesystem through these, never directly.
type Services struct {
	Config      *config.Config
	Manager     *nginx.Manager
	HostService *hosts.Service
	Certs       string // certificate directory, screens call certificates.Scan themselves
	AccessLists *accesslist.Store
	ErrorPages  *errorpages.Service
	IPLists     *iplist.Service
	RealIP      *realip.Service
	Presets     *presets.Service
	Maintenance *maintenance.Service
	Upstreams   *upstreams.Service
	Streams     *streams.Service
	GeoBlock    *geoblock.Service
	History     *history.History // nil when disabled or git is missing
	RateLimits  *ratelimit.Service
	SystemSvc   *system.Service
	Audit       *audit.Logger
	Backups     *backup.Backuper
	// Snippets is the shared write -> nginx -t -> reload workflow of every
	// standalone file.
	Snippets *snippet.Writer
}

// Build loads the configuration at cfgPath and wires every service.
func Build(cfgPath string) (*Services, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading configuration: %w", err)
	}

	renderer, err := nginx.NewRenderer(cfg.Logs.NginxDirectory, cfg.Logs.NginxDirectory, cfg.AccessLists.Directory, cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		return nil, fmt.Errorf("initializing template renderer: %w", err)
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
		return nil, fmt.Errorf("initializing audit log: %w", err)
	}

	// Nginx workers, not the root master process, read htpasswd files, so
	// they must be owned by the worker group.
	if cfg.AccessLists.Group == "" {
		group, err := nginx.DetectWorkerGroup(context.Background(), cfg.Nginx.BinaryPath, cfg.Nginx.ConfigDir)
		if err != nil {
			return nil, fmt.Errorf("%w; set access_lists.group in %s", err, cfgPath)
		}
		cfg.AccessLists.Group = group
	}
	accessLists, err := accesslist.NewStore(cfg.AccessLists.Directory, cfg.AccessLists.Group)
	if err != nil {
		return nil, fmt.Errorf("initializing access list store: %w", err)
	}

	errorPagesStore, err := errorpages.NewStore(cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		return nil, fmt.Errorf("initializing error pages store: %w", err)
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
		return nil, fmt.Errorf("initializing IP access list store: %w", err)
	}

	streamService := &streams.Service{
		Dir: cfg.Nginx.StreamsDir, MainIncludeDir: cfg.Nginx.MainIncludeDir, ConfigDir: cfg.Nginx.ConfigDir,
		SnippetsDir: cfg.ErrorPages.SnippetsDirectory, Writer: snippetWriter,
	}

	var hist *history.History
	if cfg.History.Enabled && history.Available() {
		hist = &history.History{
			GitDir:   cfg.History.GitDir,
			WorkTree: cfg.Nginx.ConfigDir,
			Paths: history.TrackedPaths(cfg.Nginx.ConfigDir,
				cfg.Nginx.SitesAvailable, cfg.Nginx.SitesEnabled, cfg.ErrorPages.SnippetsDirectory,
				cfg.Nginx.HTTPSnippetsDir, cfg.Nginx.StreamsDir,
				filepath.Join(cfg.Nginx.MainIncludeDir, streams.WrapperFileName)),
		}
		// The first run records the configuration as it was before GoNix
		// changed anything.
		if log, err := hist.Log(context.Background(), 1); err == nil && len(log) == 0 {
			_ = hist.Commit(context.Background(), "Initial state")
		}
		auditLogger.OnSuccess = func(e audit.Entry) {
			_ = hist.Commit(context.Background(), history.Message(e))
		}
	}

	return &Services{
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
		History:     hist,
		SystemSvc:   systemSvc,
		Audit:       auditLogger,
		Backups:     backups,
		Snippets:    snippetWriter,
	}, nil
}
