package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mrmmg/gonix/internal/accesslist"
	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/npm"
	"github.com/mrmmg/gonix/internal/streams"
)

func init() {
	register(command{path: "npm-import", args: "--data DIR [--letsencrypt DIR] [--disabled]",
		summary: "Import hosts, redirects and streams from Nginx Proxy Manager", write: true, run: npmImport})
}

func npmImport(e *env, args []string) error {
	fs := flag.NewFlagSet("npm-import", flag.ContinueOnError)
	data := fs.String("data", "", "NPM's data directory (mounted at /data in its container)")
	le := fs.String("letsencrypt", "", "NPM's /etc/letsencrypt directory, to copy certificates")
	disabled := fs.Bool("disabled", false, "import hosts without enabling them")
	if err := noPositional(fs, args); err != nil {
		return err
	}
	if *data == "" {
		return usageErr("--data is required")
	}
	items, err := npm.Scan(*data, *le)
	if err != nil {
		return err
	}
	npm.Prepare(items, npm.Target{CertsDir: e.svc.Config.Certificates.Directory, Exists: func(name string) bool {
		_, err := e.findHost(name)
		return err == nil
	}})

	ctx := context.Background()
	copyFile := npm.CopyFile
	if e.DryRun {
		copyFile = func(src, dst string, _ os.FileMode) error {
			e.plan.RecordNote("copy " + src + " to " + dst)
			return nil
		}
	}
	r := &npm.Runner{
		CopyFile: copyFile, CertsDir: e.svc.Config.Certificates.Directory, AccessDir: e.svc.Config.AccessLists.Directory,
		CreateIPList: func(ctx context.Context, name string, allow bool, entries []string) error {
			if e.svc.IPLists.Store.Exists(name) {
				return nil
			}
			mode := iplist.Blocklist
			if allow {
				mode = iplist.Allowlist
			}
			return e.svc.IPLists.Create(ctx, iplist.List{Name: name, Mode: mode, Entries: entries})
		},
		EnsurePreset: e.svc.Presets.Ensure,
		CreateHost: func(ctx context.Context, h nginx.Host) error {
			if _, err := e.svc.HostService.CreateHost(ctx, h); err != nil {
				return err
			}
			if *disabled {
				return nil
			}
			return e.svc.HostService.EnableHost(ctx, h.ServerName)
		},
		SaveStream: func(ctx context.Context, p streams.Proxy) error { return e.svc.Streams.Save(ctx, p, true) },
	}
	res := r.Run(ctx, items)
	if !e.DryRun {
		// Copied htpasswd files need the Nginx worker group.
		if _, err := accesslist.NewStore(e.svc.Config.AccessLists.Directory, e.svc.Config.AccessLists.Group); err != nil {
			res.Warnings = append(res.Warnings, "fixing access list permissions: "+err.Error())
		}
	}
	if e.DryRun {
		for _, s := range res.Skipped {
			e.plan.RecordNote("skip " + s)
		}
		for _, w := range res.Warnings {
			e.plan.RecordNote("warning: " + w)
		}
		return e.printPlan()
	}
	return e.show(map[string]any{"ok": len(res.Imported) > 0 || len(res.Skipped) == 0, "imported": res.Imported, "skipped": res.Skipped, "warnings": res.Warnings},
		func(w io.Writer) {
			fmt.Fprintf(w, "Imported %d item(s): %s\n", len(res.Imported), strings.Join(res.Imported, ", "))
			for _, s := range res.Skipped {
				fmt.Fprintln(w, "skipped:", s)
			}
			for _, s := range res.Warnings {
				fmt.Fprintln(w, "warning:", s)
			}
		})
}

func noPositional(fs *flag.FlagSet, args []string) error {
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	return exactArgs(pos, 0)
}
