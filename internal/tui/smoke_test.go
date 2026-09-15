package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shiva/nginx-manager/internal/accesslist"
	"github.com/shiva/nginx-manager/internal/audit"
	"github.com/shiva/nginx-manager/internal/backup"
	"github.com/shiva/nginx-manager/internal/config"
	"github.com/shiva/nginx-manager/internal/hosts"
	"github.com/shiva/nginx-manager/internal/nginx"
	"github.com/shiva/nginx-manager/internal/system"
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

	renderer, err := nginx.NewRenderer(t.TempDir(), t.TempDir(), cfg.AccessLists.Directory)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	manager := nginx.NewManager(cfg.Nginx.SitesAvailable, cfg.Nginx.SitesEnabled, renderer)
	auditLogger, err := audit.NewLogger(t.TempDir() + "/audit.log")
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	accessLists, err := accesslist.NewStore(cfg.AccessLists.Directory)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
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

	return Deps{
		Config:      cfg,
		Manager:     manager,
		HostService: svc,
		AccessLists: accessLists,
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
