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

	"github.com/mrmmg/gonix/internal/accesslist"
	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/lint"
	"github.com/mrmmg/gonix/internal/logs"
	"github.com/mrmmg/gonix/internal/transfer"
)

func init() {
	register(
		command{path: "nginx status", summary: "Show whether Nginx is running", run: nginxStatus},
		command{path: "nginx test", summary: "Run nginx -t", run: nginxTest},
		command{path: "nginx reload", summary: "Test and reload Nginx", run: nginxReload},
		command{path: "check", args: "[--no-upstreams]", summary: "Look for configuration problems (Config Check)", run: check},
		command{path: "logs", args: "<domain> [--lines N] [--filter \"status:5xx ip:... path:...\"] [--summary] [--error]",
			summary: "Show a host's recent requests or errors", run: logsCmd},
		command{path: "audit", args: "[--lines N]", summary: "Show the latest audited changes", run: auditCmd},
		command{path: "history list", args: "[--lines N]", summary: "List configuration history commits", run: historyList},
		command{path: "history show", args: "<commit>", summary: "Show a commit's diff", run: historyShow},
		command{path: "history restore", args: "<commit> --yes", summary: "Restore the configuration to a commit", run: historyRestore},
		command{path: "export", args: "<file.tar.gz | ->", summary: "Export everything GoNix manages", run: exportCmd},
		command{path: "import", args: "<file.tar.gz> [--overwrite --yes]", summary: "Import an export (e.g. on a new server)", write: true, run: importCmd},
	)
}

func noFlags(name string, args []string) error {
	pos, err := parseFlags(flag.NewFlagSet(name, flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	return exactArgs(pos, 0)
}

func nginxStatus(e *env, args []string) error {
	if err := noFlags("nginx status", args); err != nil {
		return err
	}
	st, err := e.svc.SystemSvc.GetStatus(context.Background())
	if err != nil {
		return err
	}
	b := e.svc.Manager.Renderer.Build
	out := map[string]any{"running": st.Active, "enabled_at_boot": st.Enabled,
		"version": fmt.Sprintf("%d.%d.%d", b.Major, b.Minor, b.Patch), "http3": b.SupportsHTTP3()}
	return e.show(out, func(w io.Writer) {
		fmt.Fprintf(w, "running: %v\nenabled at boot: %v\nversion: %s\n", st.Active, st.Enabled, out["version"])
	})
}

func nginxTest(e *env, args []string) error {
	if err := noFlags("nginx test", args); err != nil {
		return err
	}
	res, err := e.svc.HostService.Tester.Test(context.Background())
	if err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("nginx configuration test failed:\n%s", res.Output)
	}
	return e.show(map[string]any{"ok": true, "output": res.Output}, func(w io.Writer) { fmt.Fprint(w, res.Output) })
}

func nginxReload(e *env, args []string) error {
	if err := noFlags("nginx reload", args); err != nil {
		return err
	}
	ctx := context.Background()
	res, err := e.svc.HostService.Tester.Test(ctx)
	if err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("not reloading, nginx configuration test failed:\n%s", res.Output)
	}
	if err := e.svc.SystemSvc.Reload(ctx); err != nil {
		return err
	}
	return e.done("Nginx reloaded.")
}

func check(e *env, args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	noUpstreams := fs.Bool("no-upstreams", false, "do not dial upstreams")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	c := &lint.Checker{
		Manager: e.svc.Manager, ConfigDir: e.svc.Config.Nginx.ConfigDir, HTTPDir: e.svc.Config.Nginx.HTTPSnippetsDir,
		SnippetsDir: e.svc.Config.ErrorPages.SnippetsDirectory, AccessListDir: e.svc.Config.AccessLists.Directory,
		CheckUpstreams: !*noUpstreams,
	}
	findings, err := c.Run(context.Background())
	if err != nil {
		return err
	}
	type findingJSON struct {
		Severity string `json:"severity"`
		Target   string `json:"target"`
		Message  string `json:"message"`
	}
	out := []findingJSON{}
	errors := 0
	for _, f := range findings {
		out = append(out, findingJSON{strings.ToLower(f.Severity.String()), f.Target, f.Message})
		if f.Severity == lint.Error {
			errors++
		}
	}
	if err := e.show(out, func(w io.Writer) {
		if len(out) == 0 {
			fmt.Fprintln(w, "No problems found.")
		}
		for _, f := range findings {
			fmt.Fprintf(w, "%-5s %s: %s\n", f.Severity, f.Target, f.Message)
		}
	}); err != nil {
		return err
	}
	if errors > 0 && !e.JSON {
		return fmt.Errorf("%d error(s) found", errors)
	}
	return nil
}

func logsCmd(e *env, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	lines := fs.Int("lines", 50, "how many requests to show")
	filter := fs.String("filter", "", "status:5xx ip:1.2.3.4 path:/api")
	summary := fs.Bool("summary", false, "summarize instead of listing")
	errLog := fs.Bool("error", false, "show the error log")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 1); err != nil {
		return err
	}
	h, err := e.findHost(pos[0])
	if err != nil {
		return err
	}
	dir := e.svc.Config.Logs.NginxDirectory
	if *errLog {
		raw, err := logs.Tail(logs.ErrorLogPath(dir, h.ServerName), *lines)
		if err != nil {
			return err
		}
		return e.show(raw, func(w io.Writer) { fmt.Fprintln(w, strings.Join(raw, "\n")) })
	}
	f, err := logs.ParseFilter(*filter)
	if err != nil {
		return usageErr("%v", err)
	}
	raw, err := logs.Tail(logs.AccessLogPath(dir, h.ServerName), 5000)
	if err != nil {
		return err
	}
	var entries []logs.Entry
	for _, l := range raw {
		if en, ok := logs.ParseAccessLine(l); ok && f.Match(en) {
			entries = append(entries, en)
		}
	}
	if *summary {
		s := logs.Summarize(entries, 10)
		return e.show(s, func(w io.Writer) {
			fmt.Fprintf(w, "requests: %d (%.1f/min)  2xx %d  3xx %d  4xx %d  5xx %d\n", s.Total, s.PerMinute,
				s.ByClass["2xx"], s.ByClass["3xx"], s.ByClass["4xx"], s.ByClass["5xx"])
			for _, sec := range []struct {
				title string
				c     []logs.Count
			}{{"top clients", s.TopIPs}, {"top paths", s.TopPaths}, {"5xx paths", s.Top5xx}} {
				fmt.Fprintln(w, "\n"+sec.title+":")
				for _, c := range sec.c {
					fmt.Fprintf(w, "  %6d  %s\n", c.N, c.Value)
				}
			}
		})
	}
	if len(entries) > *lines {
		entries = entries[len(entries)-*lines:]
	}
	type entryJSON struct {
		Time      time.Time `json:"time"`
		IP        string    `json:"ip"`
		Method    string    `json:"method"`
		Path      string    `json:"path"`
		Status    int       `json:"status"`
		Bytes     int64     `json:"bytes"`
		UserAgent string    `json:"user_agent"`
	}
	out := []entryJSON{}
	for _, en := range entries {
		out = append(out, entryJSON{en.Time, en.IP, en.Method, en.Path, en.Status, en.Bytes, en.UserAgent})
	}
	return e.show(out, func(w io.Writer) {
		for _, en := range entries {
			fmt.Fprintln(w, en.Raw)
		}
	})
}

func auditCmd(e *env, args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	lines := fs.Int("lines", 20, "how many entries")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	raw, err := logs.Tail(e.svc.Audit.Path(), *lines)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	type entryJSON struct {
		Time   time.Time `json:"time"`
		User   string    `json:"user"`
		Actor  string    `json:"actor,omitempty"`
		Action string    `json:"action"`
		Target string    `json:"target"`
		Result string    `json:"result"`
		Error  string    `json:"error,omitempty"`
	}
	out := []entryJSON{}
	for _, l := range raw {
		if en, ok := audit.ParseLine(l); ok {
			out = append(out, entryJSON{en.Timestamp, en.User, en.Actor, en.Action, en.Target, string(en.Result), en.Error})
		}
	}
	return e.show(out, func(w io.Writer) { fmt.Fprintln(w, strings.Join(raw, "\n")) })
}

func (e *env) requireHistory() error {
	if e.svc.History == nil {
		return fmt.Errorf("configuration history is disabled (install git and set history.enabled: true)")
	}
	return nil
}

func historyList(e *env, args []string) error {
	fs := flag.NewFlagSet("history list", flag.ContinueOnError)
	lines := fs.Int("lines", 20, "how many commits")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := e.requireHistory(); err != nil {
		return err
	}
	commits, err := e.svc.History.Log(context.Background(), *lines)
	if err != nil {
		return err
	}
	type commitJSON struct {
		Hash    string    `json:"hash"`
		Time    time.Time `json:"time"`
		Message string    `json:"message"`
	}
	out := []commitJSON{}
	for _, c := range commits {
		out = append(out, commitJSON{c.Hash, c.Time, c.Message})
	}
	return e.show(out, func(w io.Writer) {
		for _, c := range commits {
			fmt.Fprintf(w, "%s  %s  %s\n", c.Hash[:10], c.Time.Format("2006-01-02 15:04:05"), c.Message)
		}
	})
}

func historyShow(e *env, args []string) error {
	name, err := nameArg(flag.NewFlagSet("history show", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := e.requireHistory(); err != nil {
		return err
	}
	out, err := e.svc.History.Show(context.Background(), name)
	if err != nil {
		return err
	}
	return e.show(map[string]string{"commit": name, "diff": out}, func(w io.Writer) { fmt.Fprint(w, out) })
}

func historyRestore(e *env, args []string) error {
	name, err := nameArg(flag.NewFlagSet("history restore", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := e.requireHistory(); err != nil {
		return err
	}
	if err := e.confirm("restoring the configuration to " + name); err != nil {
		return err
	}
	ctx := context.Background()
	err = e.svc.History.Restore(ctx, name, e.svc.HostService.Tester)
	if err == nil {
		err = e.svc.SystemSvc.Reload(ctx)
	}
	entry := audit.Entry{User: audit.CurrentUser(), Action: "history_restored", Target: name, Result: audit.ResultSuccess}
	if err != nil {
		entry.Result, entry.Error = audit.ResultFailure, err.Error()
	}
	_ = e.svc.Audit.Log(entry)
	if err != nil {
		return err
	}
	return e.done("Configuration restored to " + name + " and Nginx reloaded.")
}

func (e *env) transferDirs() transfer.Dirs {
	c := e.svc.Config
	return transfer.Dirs{
		SitesAvailable: c.Nginx.SitesAvailable, SitesEnabled: c.Nginx.SitesEnabled, Snippets: c.ErrorPages.SnippetsDirectory,
		HTTP: c.Nginx.HTTPSnippetsDir, AccessLists: c.AccessLists.Directory, Streams: c.Nginx.StreamsDir, Main: c.Nginx.MainIncludeDir,
	}
}

func exportCmd(e *env, args []string) error {
	name, err := nameArg(flag.NewFlagSet("export", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	var w io.Writer = e.out
	if name != "-" {
		f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	} else if e.JSON {
		return usageErr("--json cannot be combined with exporting to stdout")
	}
	bw := bufio.NewWriter(w)
	m, err := transfer.Export(bw, e.transferDirs(), e.svc.Manager)
	if err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if name == "-" {
		return nil
	}
	return e.show(map[string]any{"ok": true, "file": name, "enabled_hosts": m.Enabled}, func(w io.Writer) {
		fmt.Fprintf(w, "Exported to %s (contains access list password hashes; keep it private).\n", name)
	})
}

func importCmd(e *env, args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	overwrite := fs.Bool("overwrite", false, "replace existing files that differ")
	name, err := nameArg(fs, args)
	if err != nil {
		return err
	}
	if *overwrite {
		if err := e.confirm("overwriting existing files"); err != nil {
			return err
		}
	}
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	plan, err := transfer.Read(f, e.transferDirs())
	if err != nil {
		return err
	}
	im := &transfer.Importer{Dirs: e.transferDirs(), Manager: e.svc.Manager, Writer: e.svc.Snippets,
		AfterWrite: func() error {
			_, err := accesslist.NewStore(e.svc.Config.AccessLists.Directory, e.svc.Config.AccessLists.Group)
			return err
		}}
	if err := im.Apply(context.Background(), plan, *overwrite); err != nil {
		return err
	}
	return e.done(fmt.Sprintf("Imported %d file(s) from %s (exported on %s); %d host(s) enabled.",
		len(plan.Files), name, plan.Manifest.Hostname, len(plan.Enable)))
}
