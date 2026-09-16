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
	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/system"
	"github.com/mrmmg/gonix/internal/tui"
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
