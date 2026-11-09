package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/exporter"
	"github.com/mrmmg/gonix/internal/update"
)

func init() {
	register(command{path: "update", args: "[--check] [--rollback] [--yes]",
		summary: "Update GoNix from its latest GitHub release (checksum verified, previous version kept)", run: updateCmd})
}

// newUpdateClient and executablePath are replaced by tests.
var (
	newUpdateClient = update.NewClient
	executablePath  = os.Executable
)

func updateCmd(e *env, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	checkOnly := fs.Bool("check", false, "only report whether a newer release exists")
	rollback := fs.Bool("rollback", false, "put the version kept by the last update back")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 0); err != nil {
		return err
	}
	cfg, err := config.Load(e.cfgPath)
	if err != nil {
		return err
	}
	exe, err := executablePath()
	if err != nil {
		return fmt.Errorf("finding the running gonix: %w", err)
	}
	if !*checkOnly && RequireRoot && os.Geteuid() != 0 {
		return fmt.Errorf("updating replaces %s and needs root; run it with sudo", exe)
	}

	if *rollback {
		if err := e.confirmUpdate("Put the previous version of gonix back?"); err != nil {
			return err
		}
		if err := update.Rollback(exe); err != nil {
			return err
		}
		logUpdate(cfg, "gonix_update_rolled_back", exe, nil)
		restartExporters(e, cfg, exe)
		return e.done("Rolled back. The version that was installed before is kept as " + update.BackupPath(exe) + ".")
	}

	ctx := context.Background()
	client := newUpdateClient()
	rel, err := client.Check(ctx, update.CachePath(cfg.Logs.AuditFile), true)
	if err != nil {
		return fmt.Errorf("checking GitHub (%s): %w", update.ReleasesURL, err)
	}
	newer := update.Newer(rel.Tag, Version)
	if *checkOnly || !newer {
		out := map[string]any{"current": Version, "latest": rel.Version, "update_available": newer, "url": rel.URL, "notes": rel.Notes}
		return e.show(out, func(w io.Writer) {
			if !newer {
				fmt.Fprintf(w, "gonix %s is up to date (latest release: %s).\n", Version, rel.Version)
				return
			}
			fmt.Fprintf(w, "gonix %s is available (you have %s).\n%s\n", rel.Version, Version, rel.URL)
			if rel.Notes != "" {
				fmt.Fprintf(w, "\n%s\n", rel.Notes)
			}
			fmt.Fprintln(w, "\nUpdate with: sudo gonix update")
		})
	}

	if err := e.confirmUpdate(fmt.Sprintf("Update gonix %s → %s from %s?", Version, rel.Version, update.ReleasesURL)); err != nil {
		return err
	}
	progress := func(s string) {
		if !e.JSON {
			fmt.Fprintln(e.out, s)
		}
	}
	// The exporter's unit must keep pointing at this path, which
	// os.Executable no longer reports once the file has been replaced.
	err = client.Apply(ctx, exe, rel, progress)
	logUpdate(cfg, "gonix_updated", rel.Version, err)
	if err != nil {
		return err
	}
	restartExporters(e, cfg, exe)
	return e.done(fmt.Sprintf("Updated gonix %s → %s. The previous version is kept as %s (undo with: sudo gonix update --rollback).",
		Version, rel.Version, update.BackupPath(exe)))
}

// confirmUpdate asks before replacing the binary, unless --yes was given.
func (e *env) confirmUpdate(question string) error {
	if e.Yes {
		return nil
	}
	if e.JSON {
		return fmt.Errorf("add --yes to update without a prompt")
	}
	fmt.Fprintf(e.out, "%s [y/N] ", question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return fmt.Errorf("cancelled (use --yes to skip this question)")
	}
	return nil
}

func logUpdate(cfg *config.Config, action, target string, err error) {
	l, lerr := audit.NewLogger(cfg.Logs.AuditFile)
	if lerr != nil {
		return
	}
	en := audit.Entry{Timestamp: time.Now(), User: audit.CurrentUser(), Action: action, Target: target, Result: audit.ResultSuccess, Actor: "cli"}
	if err != nil {
		en.Result, en.Error = audit.ResultFailure, err.Error()
	}
	_ = l.Log(en)
}

// restartExporters restarts the running Prometheus exporter of this
// instance and of every other one, if the settings ask for it, so they run
// the new binary. Exporters that are not running are left alone.
func restartExporters(e *env, cfg *config.Config, exe string) {
	if !cfg.Update.RestartExporter {
		return
	}
	paths := []string{e.cfgPath}
	if cfg.Instance == "" {
		if names, err := config.ListInstances(config.InstancesDir); err == nil {
			for _, n := range names {
				if p, err := config.InstancePath(n); err == nil {
					paths = append(paths, p)
				}
			}
		}
	}
	ctx := context.Background()
	for _, p := range paths {
		c, err := config.Load(p)
		if err != nil {
			continue
		}
		svc := exporter.New(c, p, nil)
		svc.Binary = exe
		if !svc.Status(ctx).Active {
			continue
		}
		if err := svc.Restart(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "gonix: could not restart %s: %v\n", svc.UnitName(), err)
			continue
		}
		if !e.JSON {
			fmt.Fprintf(e.out, "Restarted %s.\n", svc.UnitName())
		}
	}
}
