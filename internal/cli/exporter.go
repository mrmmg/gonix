package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/dashboard"
	"github.com/mrmmg/gonix/internal/metrics"
)

func init() {
	register(
		command{path: "exporter run", summary: "Serve Prometheus metrics in the foreground (what the systemd unit runs)", run: exporterRun},
		command{path: "exporter status", summary: "Show the exporter service and its settings", run: exporterStatus},
		command{path: "exporter enable", summary: "Install the systemd unit, then start the exporter now and at boot", run: exporterAction("enable")},
		command{path: "exporter disable", summary: "Stop the exporter and keep it from starting at boot", run: exporterAction("disable")},
		command{path: "exporter start", summary: "Start the exporter", run: exporterAction("start")},
		command{path: "exporter stop", summary: "Stop the exporter", run: exporterAction("stop")},
		command{path: "exporter restart", summary: "Restart the exporter", run: exporterAction("restart")},
		command{path: "exporter uninstall", summary: "Stop the exporter and remove its systemd unit (settings stay)", run: exporterAction("uninstall")},
		command{path: "exporter set",
			args:    "[--listen ADDR] [--hosts P,..|none] [--exclude-hosts P,..|none] [--collectors C,..|all] [--cache-ttl 30s] [--stub-status on|off] [--stub-status-listen ADDR]",
			summary: "Change the exporter's settings (restarts it if running)", run: exporterSet},
		command{path: "exporter dashboard", args: "[--out FILE|DIR]",
			summary: "Write the Grafana dashboard for these metrics (to a file, or stdout)", run: exporterDashboard},
		command{path: "exporter metrics", args: "[--host P]... [--exclude-host P]... [--collect NAME]...",
			summary: "Print one scrape of the metrics (to check them without Prometheus)", run: exporterMetrics},
	)
}

func exporterRun(e *env, args []string) error {
	if err := noFlags("exporter run", args); err != nil {
		return err
	}
	ex := metrics.FromServices(e.svc, Version)
	defer ex.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	addr := e.svc.Config.Monitoring.Listen
	fmt.Fprintf(os.Stderr, "gonix exporter: serving http://%s/metrics (instance %q)\n", addr, e.svc.Config.Instance)
	return ex.Serve(ctx, addr)
}

type exporterJSON struct {
	Unit      string                  `json:"unit"`
	Installed bool                    `json:"installed"`
	UnitStale bool                    `json:"unit_stale"`
	Running   bool                    `json:"running"`
	State     string                  `json:"state"`
	Enabled   bool                    `json:"enabled_at_boot"`
	URL       string                  `json:"url"`
	Settings  config.MonitoringConfig `json:"settings"`
}

func exporterStatus(e *env, args []string) error {
	if err := noFlags("exporter status", args); err != nil {
		return err
	}
	st := e.svc.Exporter.Status(context.Background())
	m := e.svc.Config.Monitoring
	out := exporterJSON{st.Unit, st.Installed, st.UnitStale, st.Active, st.State, st.Enabled, st.URL, m}
	return e.show(out, func(w io.Writer) {
		fmt.Fprintf(w, "unit:       %s (%s)\n", st.Unit, map[bool]string{true: "installed", false: "not installed; run 'gonix exporter enable'"}[st.Installed])
		fmt.Fprintf(w, "running:    %v (%s)\nat boot:    %v\nurl:        %s\n", st.Active, st.State, st.Enabled, st.URL)
		if st.UnitStale {
			fmt.Fprintln(w, "note:       the unit differs from what this GoNix would write; 'gonix exporter restart' updates it")
		}
		fmt.Fprintf(w, "hosts:      %s\nexclude:    %s\ncollectors: %s\ncache ttl:  %s\nstub status: %v (%s)\n",
			orAll(m.Hosts), orNone(m.ExcludeHosts), orAll(m.Collectors), m.CacheTTL, m.StubStatus.Enabled, m.StubStatus.Listen)
	})
}

func orAll(v []string) string {
	if len(v) == 0 {
		return "all"
	}
	return strings.Join(v, ", ")
}

func orNone(v []string) string {
	if len(v) == 0 {
		return "none"
	}
	return strings.Join(v, ", ")
}

func exporterAction(name string) func(*env, []string) error {
	return func(e *env, args []string) error {
		if err := noFlags("exporter "+name, args); err != nil {
			return err
		}
		x, ctx := e.svc.Exporter, context.Background()
		var err error
		var msg string
		switch name {
		case "enable":
			err, msg = x.Enable(ctx), "Exporter enabled: "+x.Status(ctx).URL
		case "disable":
			err, msg = x.Disable(ctx), "Exporter stopped and disabled."
		case "start":
			err, msg = x.Start(ctx), "Exporter started."
		case "stop":
			err, msg = x.Stop(ctx), "Exporter stopped."
		case "restart":
			err, msg = x.Restart(ctx), "Exporter restarted."
		case "uninstall":
			err, msg = x.Uninstall(ctx), "Exporter unit removed."
		}
		if err != nil {
			return err
		}
		return e.done(msg)
	}
}

// list splits a comma separated flag value; "none" and "all" mean empty.
func list(v string) []string {
	if v == "" || v == "none" || v == "all" {
		return nil
	}
	return slices.DeleteFunc(strings.Split(v, ","), func(s string) bool { return s == "" })
}

func exporterSet(e *env, args []string) error {
	fs := flag.NewFlagSet("exporter set", flag.ContinueOnError)
	listen := fs.String("listen", "", "address /metrics is served on")
	hosts := fs.String("hosts", "", "only these hosts (patterns like *.example.com, comma separated; none = all hosts)")
	exclude := fs.String("exclude-hosts", "", "leave these hosts out (none = leave none out)")
	collectors := fs.String("collectors", "", "enabled collectors: "+strings.Join(config.Collectors, ", ")+" (all = every one)")
	ttl := fs.Duration("cache-ttl", 0, "how long slow results are reused")
	stub := fs.String("stub-status", "", "on|off: serve Nginx connection counters (adds a local-only server block)")
	stubListen := fs.String("stub-status-listen", "", "address of that server block")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 0); err != nil {
		return err
	}
	set := setFlags(fs)
	if len(set) == 0 {
		return usageErr("nothing to change")
	}
	m := e.svc.Config.Monitoring
	if set["listen"] {
		m.Listen = *listen
	}
	if set["hosts"] {
		m.Hosts = list(*hosts)
	}
	if set["exclude-hosts"] {
		m.ExcludeHosts = list(*exclude)
	}
	if set["collectors"] {
		m.Collectors = list(*collectors)
	}
	if set["cache-ttl"] {
		m.CacheTTL = *ttl
	}
	if set["stub-status-listen"] {
		m.StubStatus.Listen = *stubListen
	}
	if set["stub-status"] {
		switch *stub {
		case "on":
			m.StubStatus.Enabled = true
		case "off":
			m.StubStatus.Enabled = false
		default:
			return usageErr("--stub-status takes on or off")
		}
	}
	if err := e.svc.Exporter.Apply(context.Background(), m); err != nil {
		return err
	}
	return e.done("Exporter settings saved.")
}

type multiFlag = stringsFlag

func exporterMetrics(e *env, args []string) error {
	fs := flag.NewFlagSet("exporter metrics", flag.ContinueOnError)
	var hosts, exclude, collect multiFlag
	fs.Var(&hosts, "host", "only this host (pattern); repeatable")
	fs.Var(&exclude, "exclude-host", "leave this host out (pattern); repeatable")
	fs.Var(&collect, "collect", "only this collector; repeatable")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 0); err != nil {
		return err
	}
	ex := metrics.FromServices(e.svc, Version)
	defer ex.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return ex.Write(ctx, e.out, metrics.Filter{Include: hosts, Exclude: exclude}, collect)
}

func exporterDashboard(e *env, args []string) error {
	fs := flag.NewFlagSet("exporter dashboard", flag.ContinueOnError)
	out := fs.String("out", "", "file to write, or a directory (gonix-dashboard.json inside it); stdout if empty")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := exactArgs(pos, 0); err != nil {
		return err
	}
	if *out == "" {
		data, err := dashboard.JSON()
		if err != nil {
			return err
		}
		_, err = e.out.Write(data)
		return err
	}
	path, err := dashboard.Save(*out)
	if err != nil {
		return err
	}
	if e.JSON {
		return writeJSON(e.out, map[string]any{"ok": true, "path": path})
	}
	fmt.Fprintf(e.out, "Dashboard written to %s (import it in Grafana: Dashboards, New, Import)\n", path)
	return nil
}
