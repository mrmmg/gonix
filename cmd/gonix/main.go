// Command GoNix is an interactive terminal UI for managing Nginx
// virtual hosts, TLS certificates, access lists and the Nginx service
// itself, without hiding the underlying, human-editable Nginx configuration.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/acme"
	"github.com/mrmmg/gonix/internal/app"
	"github.com/mrmmg/gonix/internal/cli"
	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/tui"
)

func main() {
	cli.Version = tui.Version
	// "gonix --instance staging" opens the TUI for that instance; CLI
	// commands handle --instance themselves.
	if !cli.IsCommand(os.Args[1:]) {
		os.Args = append(os.Args[:1], takeInstanceFlag(os.Args[1:])...)
	}
	switch {
	case len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v"):
		fmt.Println("gonix version " + tui.Version)
		return
	case len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h"):
		os.Exit(cli.Run([]string{"help"}, os.Stdout, os.Stderr, app.Build, configPath()))
	case cli.IsCommand(os.Args[1:]) || (len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-")):
		os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, app.Build, configPath()))
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

// configPath is the configuration file: GONIX_CONFIG, or the instance
// named by GONIX_INSTANCE, else the default.
func configPath() string {
	if v := os.Getenv("GONIX_CONFIG"); v != "" {
		return v
	}
	if v := os.Getenv("GONIX_INSTANCE"); v != "" {
		if p, err := config.InstancePath(v); err == nil {
			return p
		}
	}
	return config.DefaultPath
}

// takeInstanceFlag removes "--instance NAME" from the TUI's arguments and
// selects that instance.
func takeInstanceFlag(args []string) []string {
	var rest []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--instance" && i+1 < len(args):
			os.Setenv("GONIX_INSTANCE", args[i+1])
			i++
		case strings.HasPrefix(args[i], "--instance="):
			os.Setenv("GONIX_INSTANCE", strings.TrimPrefix(args[i], "--instance="))
		default:
			rest = append(rest, args[i])
		}
	}
	return rest
}

func run() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this operation requires root privileges.\nPlease run gonix with sudo:\n\n    sudo gonix")
	}

	deps, err := app.Build(configPath())
	if err != nil {
		return err
	}

	p := tea.NewProgram(tui.NewApp(*deps), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
