// Package cli is GoNix's non-interactive command line: the same services
// as the TUI (and the same backup, nginx -t, rollback and audit guarantees)
// for scripts, CI and AI agents. Every read command can print JSON, and
// every write command can preview its exact changes with --dry-run.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mrmmg/gonix/internal/app"
	"github.com/mrmmg/gonix/internal/diff"
	"github.com/mrmmg/gonix/internal/snippet"
)

// Options are the global flags, accepted anywhere on the command line.
type Options struct {
	JSON   bool
	DryRun bool
	Yes    bool
	Config string
}

// Builder loads the services for a configuration file path.
type Builder func(cfgPath string) (*app.Services, error)

// RequireRoot makes commands that need the configuration refuse to run
// without root. Tests turn it off.
var RequireRoot = true

// errUsage marks errors caused by how the command was invoked.
var errUsage = errors.New("usage")

func usageErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, args...))
}

// env is what a command runs with.
type env struct {
	Options
	out  io.Writer
	svc  *app.Services
	plan *snippet.Plan
}

// command is one leaf of the command tree.
type command struct {
	path    string // e.g. "host add"
	args    string // positional arguments, for help
	summary string
	write   bool // changes configuration; supports --dry-run
	run     func(e *env, args []string) error
}

var commands []command

func register(cs ...command) { commands = append(commands, cs...) }

// IsCommand reports whether args name a CLI command (as opposed to TUI
// flags such as --version).
func IsCommand(args []string) bool {
	_, rest := splitGlobals(args)
	if len(rest) == 0 {
		return false
	}
	if rest[0] == "help" {
		return true
	}
	for _, c := range commands {
		if strings.Fields(c.path)[0] == rest[0] {
			return true
		}
	}
	return false
}

// splitGlobals removes the global flags from args.
func splitGlobals(args []string) (Options, []string) {
	var o Options
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			o.JSON = true
		case a == "--dry-run":
			o.DryRun = true
		case a == "--yes" || a == "-y":
			o.Yes = true
		case a == "--config" && i+1 < len(args):
			o.Config = args[i+1]
			i++
		case strings.HasPrefix(a, "--config="):
			o.Config = strings.TrimPrefix(a, "--config=")
		default:
			rest = append(rest, a)
		}
	}
	return o, rest
}

// find returns the command whose path is the longest prefix of args.
func find(args []string) (command, []string, bool) {
	best, n := command{}, 0
	for _, c := range commands {
		words := strings.Fields(c.path)
		if len(words) <= len(args) && len(words) > n && strings.Join(args[:len(words)], " ") == c.path {
			best, n = c, len(words)
		}
	}
	return best, args[n:], n > 0
}

// Run executes the command in args and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer, build Builder, defaultConfig string) int {
	opts, rest := splitGlobals(args)
	if len(rest) == 0 || rest[0] == "help" {
		printHelp(stdout, rest)
		return 0
	}
	cmd, cmdArgs, ok := find(rest)
	if !ok {
		fmt.Fprintf(stderr, "gonix: unknown command %q (see gonix help)\n", strings.Join(rest, " "))
		return 2
	}
	e := &env{Options: opts, out: stdout}
	fail := func(err error) int {
		if e.JSON {
			writeJSON(stdout, map[string]any{"ok": false, "error": err.Error()})
		} else {
			fmt.Fprintln(stderr, "gonix:", err)
		}
		if errors.Is(err, errUsage) {
			fmt.Fprintf(stderr, "usage: gonix %s %s\n", cmd.path, cmd.args)
			return 2
		}
		return 1
	}
	if e.DryRun && !cmd.write {
		return fail(usageErr("--dry-run only applies to commands that change the configuration"))
	}
	if cmd.run == nil {
		return fail(fmt.Errorf("not implemented"))
	}
	if needsServices(cmd) {
		if RequireRoot && os.Geteuid() != 0 {
			return fail(fmt.Errorf("this command requires root privileges; run it with sudo"))
		}
		cfg := opts.Config
		if cfg == "" {
			cfg = defaultConfig
		}
		svc, err := build(cfg)
		if err != nil {
			return fail(err)
		}
		svc.Audit.Actor = "cli"
		if e.DryRun {
			e.plan = &snippet.Plan{}
			svc.Snippets.DryRun = e.plan
			svc.HostService.DryRun = e.plan
		}
		e.svc = svc
	}
	if err := cmd.run(e, cmdArgs); err != nil {
		return fail(err)
	}
	return 0
}

// needsServices is false for commands that work without a configuration.
func needsServices(c command) bool {
	return !strings.HasPrefix(c.path, "skill")
}

// done reports a successful write: the plan in dry-run mode, else msg.
func (e *env) done(msg string) error {
	if e.DryRun {
		return e.printPlan()
	}
	if e.JSON {
		return writeJSON(e.out, map[string]any{"ok": true, "message": msg})
	}
	fmt.Fprintln(e.out, msg)
	return nil
}

// show prints v as JSON, or calls text otherwise.
func (e *env) show(v any, text func(w io.Writer)) error {
	if e.JSON {
		return writeJSON(e.out, v)
	}
	text(e.out)
	return nil
}

func (e *env) confirm(what string) error {
	if e.Yes || e.DryRun {
		return nil
	}
	return fmt.Errorf("%s needs confirmation: add --yes (or preview it with --dry-run)", what)
}

type changeJSON struct {
	Path    string `json:"path,omitempty"`
	Action  string `json:"action"` // create, modify, remove, note
	Note    string `json:"note,omitempty"`
	Diff    string `json:"diff,omitempty"`
	Content string `json:"content,omitempty"`
}

func (e *env) printPlan() error {
	var changes []changeJSON
	for _, c := range e.plan.Changes {
		j := changeJSON{Path: c.Path, Note: c.Note}
		switch {
		case c.Note != "":
			j.Action = "note"
		case c.Removed:
			j.Action = "remove"
			j.Diff = diff.Unified(c.Old, "", c.Path, "/dev/null")
		case !c.Existed:
			j.Action = "create"
			j.Content = c.New
			j.Diff = diff.Unified("", c.New, "/dev/null", c.Path)
		default:
			j.Action = "modify"
			j.Diff = diff.Unified(c.Old, c.New, c.Path, c.Path)
		}
		changes = append(changes, j)
	}
	if e.JSON {
		return writeJSON(e.out, map[string]any{"ok": true, "dry_run": true, "changes": changes})
	}
	if len(changes) == 0 {
		fmt.Fprintln(e.out, "Dry run: nothing would change.")
		return nil
	}
	fmt.Fprintf(e.out, "Dry run: %d change(s) would be made (then checked with nginx -t and reloaded):\n\n", len(changes))
	for _, c := range changes {
		if c.Action == "note" {
			fmt.Fprintf(e.out, "* %s\n\n", c.Note)
			continue
		}
		fmt.Fprintln(e.out, c.Diff)
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// parseFlags parses fs from args, allowing flags before, between and after
// positional arguments, and returns the positional ones.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				var b strings.Builder
				fs.SetOutput(&b)
				fs.PrintDefaults()
				return nil, usageErr("flags of this command:\n%s", b.String())
			}
			return nil, usageErr("%v", err)
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// setFlags returns the names of the flags given on the command line.
func setFlags(fs *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

func exactArgs(pos []string, n int) error {
	if len(pos) != n {
		return usageErr("expected %d argument(s), got %d", n, len(pos))
	}
	return nil
}

// stringsFlag collects a repeatable flag.
type stringsFlag []string

func (s *stringsFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringsFlag) Set(v string) error { *s = append(*s, v); return nil }

// noneOr maps "none" (and "") to "" for flags that clear a setting.
func noneOr(v string) string {
	if v == "none" {
		return ""
	}
	return v
}

func printHelp(w io.Writer, args []string) {
	prefix := strings.Join(args[min(1, len(args)):], " ")
	fmt.Fprint(w, `gonix - manage Nginx from the terminal

Usage:
  sudo gonix                    Launch the interactive TUI
  sudo gonix <command> [flags]  Run a single command (scripts, CI, AI agents)

Global flags (anywhere on the command line):
  --json        Machine-readable output (results and errors)
  --dry-run     Show the exact configuration changes without making them
  --yes, -y     Confirm destructive commands (delete, restore, overwrite)
  --config PATH Configuration file (default /etc/gonix/gonix.yaml, or GONIX_CONFIG)

Every change is backed up, validated with nginx -t, rolled back if the test
fails, reloaded, written to the audit log (actor "cli", or GONIX_ACTOR) and
committed to the configuration history.

Commands:
`)
	sorted := append([]command(nil), commands...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
	for _, c := range sorted {
		if prefix != "" && !strings.HasPrefix(c.path, prefix) {
			continue
		}
		mark := " "
		if c.write {
			mark = "*"
		}
		usage := strings.TrimSpace(c.path + " " + c.args)
		if len(usage) > 38 {
			fmt.Fprintf(w, "  %s %s\n  %-40s %s\n", mark, usage, "", c.summary)
		} else {
			fmt.Fprintf(w, "  %s %-38s %s\n", mark, usage, c.summary)
		}
	}
	fmt.Fprint(w, "\n  * changes the configuration (supports --dry-run)\n\nRun 'gonix help <command>' to list only matching commands.\n")
}
