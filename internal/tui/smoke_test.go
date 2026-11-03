package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/accesslist"
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
	"github.com/mrmmg/gonix/internal/upstreams"
)

// testDeps builds a fully wired Deps against temporary directories, so the
// TUI can be smoke-tested without root privileges or a real Nginx install.
func testDeps(t *testing.T) Deps {
	t.Helper()
	cfg := config.Default()
	cfg.Nginx.SitesAvailable = t.TempDir()
	cfg.Nginx.SitesEnabled = t.TempDir()
	cfg.Certificates.Directory = t.TempDir()
	cfg.AccessLists.Directory = t.TempDir()
	cfg.Backup.Directory = t.TempDir()
	cfg.ErrorPages.SnippetsDirectory = t.TempDir()
	cfg.ErrorPages.PagesDirectory = t.TempDir()

	renderer, err := nginx.NewRenderer(t.TempDir(), t.TempDir(), cfg.AccessLists.Directory, cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	manager := nginx.NewManager(cfg.Nginx.SitesAvailable, cfg.Nginx.SitesEnabled, renderer)
	auditLogger, err := audit.NewLogger(t.TempDir() + "/audit.log")
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	accessLists, err := accesslist.NewStore(cfg.AccessLists.Directory, cfg.AccessLists.Group)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	errorPagesStore, err := errorpages.NewStore(cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		t.Fatalf("errorpages.NewStore: %v", err)
	}
	backups := backup.New(cfg.Backup.Directory, 5)
	systemSvc := system.NewService("nginx")

	svc := &hosts.Service{
		Manager:  manager,
		Tester:   nginx.NewValidator("nginx"),
		Reloader: nil,
		Backups:  backups,
		Audit:    auditLogger,
	}

	ipListStore, err := iplist.NewStore(cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		t.Fatalf("iplist.NewStore: %v", err)
	}
	cfg.Nginx.HTTPSnippetsDir = t.TempDir()
	writer := &snippet.Writer{Tester: okTester{}, Audit: auditLogger}

	return Deps{
		Config:      cfg,
		Manager:     manager,
		HostService: svc,
		AccessLists: accessLists,
		ErrorPages: &errorpages.Service{
			Store:   errorPagesStore,
			Manager: manager,
			Tester:  nginx.NewValidator("nginx"),
			Audit:   auditLogger,
		},
		IPLists: &iplist.Service{Store: ipListStore, Manager: manager, Writer: writer},
		RealIP:  &realip.Service{Dir: cfg.Nginx.HTTPSnippetsDir, Writer: writer},
		RateLimits: &ratelimit.Service{
			SnippetsDir: cfg.ErrorPages.SnippetsDirectory, HTTPDir: cfg.Nginx.HTTPSnippetsDir,
			Manager: manager, Writer: writer,
		},
		Upstreams: &upstreams.Service{Dir: cfg.Nginx.HTTPSnippetsDir, Manager: manager, Writer: writer},
		Streams: &streams.Service{
			Dir: t.TempDir(), MainIncludeDir: t.TempDir(), ConfigDir: t.TempDir(),
			SnippetsDir: cfg.ErrorPages.SnippetsDirectory, Writer: writer,
		},
		GeoBlock: &geoblock.Service{
			HTTPDir: cfg.Nginx.HTTPSnippetsDir, SnippetsDir: cfg.ErrorPages.SnippetsDirectory,
			Database: cfg.GeoIP.Database, Manager: manager, Writer: writer,
		},
		Maintenance: &maintenance.Service{HTTPDir: cfg.Nginx.HTTPSnippetsDir, Hosts: svc, Writer: writer},
		Presets:     &presets.Service{SnippetsDir: cfg.ErrorPages.SnippetsDirectory, HTTPDir: cfg.Nginx.HTTPSnippetsDir, Writer: writer},
		SystemSvc:   systemSvc,
		Audit:       auditLogger,
		Backups:     backups,
	}
}

// TestAppNavigatesMainMenuWithoutPanicking exercises the main menu screen
// and pushes into each top-level section, ensuring the whole navigation
// stack initializes and renders without error even with an empty backing
// filesystem.
func TestAppNavigatesMainMenuWithoutPanicking(t *testing.T) {
	deps := testDeps(t)
	model := NewApp(deps)

	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = model.View()

	for i := 0; i < miExit; i++ {
		root := model.(*rootModel)
		root.stack[len(root.stack)-1] = newMainMenu(deps)
		menu := root.stack[len(root.stack)-1].(*mainMenu)
		menu.menu.cursor = i

		var cmd tea.Cmd
		model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil {
			msg := cmd()
			model, _ = model.Update(msg)
		}
		view := model.View()
		if view == "" {
			t.Errorf("item %d produced empty view", i)
		}
		// Pop back to the main menu for the next iteration.
		root = model.(*rootModel)
		root.stack = []screen{newMainMenu(deps)}
	}
}

func TestTitleShowsInstance(t *testing.T) {
	deps := testDeps(t)
	deps.Config.Instance = "staging"
	defer func() { instanceLabel = "" }()
	model := NewApp(deps)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if !strings.Contains(model.View(), "GONIX · staging") {
		t.Error("title bar does not show the instance")
	}
}
