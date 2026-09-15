// Command GoNix is an interactive terminal UI for managing Nginx
// virtual hosts, TLS certificates, access lists and the Nginx service
// itself, without hiding the underlying, human-editable Nginx configuration.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shiva/gonix/internal/accesslist"
	"github.com/shiva/gonix/internal/audit"
	"github.com/shiva/gonix/internal/backup"
	"github.com/shiva/gonix/internal/config"
	"github.com/shiva/gonix/internal/hosts"
	"github.com/shiva/gonix/internal/nginx"
	"github.com/shiva/gonix/internal/system"
	"github.com/shiva/gonix/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gonix:", err)
		os.Exit(1)
	}
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

	renderer, err := nginx.NewRenderer(cfg.Logs.NginxDirectory, cfg.Logs.NginxDirectory, cfg.AccessLists.Directory)
	if err != nil {
		return fmt.Errorf("initializing template renderer: %w", err)
	}
	manager := nginx.NewManager(cfg.Nginx.SitesAvailable, cfg.Nginx.SitesEnabled, renderer)
	validator := nginx.NewValidator(cfg.Nginx.BinaryPath)
	systemSvc := system.NewService(cfg.Nginx.BinaryPath)
	backups := backup.New(cfg.Backup.Directory, cfg.Backup.KeepCount)

	auditLogger, err := audit.NewLogger(cfg.Logs.AuditFile)
	if err != nil {
		return fmt.Errorf("initializing audit log: %w", err)
	}

	accessLists, err := accesslist.NewStore(cfg.AccessLists.Directory)
	if err != nil {
		return fmt.Errorf("initializing access list store: %w", err)
	}

	hostService := &hosts.Service{
		Manager:  manager,
		Tester:   validator,
		Reloader: systemSvc,
		Backups:  backups,
		Audit:    auditLogger,
	}

	deps := tui.Deps{
		Config:      cfg,
		Manager:     manager,
		HostService: hostService,
		AccessLists: accessLists,
		SystemSvc:   systemSvc,
		Audit:       auditLogger,
		Backups:     backups,
	}

	p := tea.NewProgram(tui.NewApp(deps), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
