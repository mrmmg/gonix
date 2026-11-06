package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/nginx"
)

func init() {
	register(
		command{path: "adopt", args: "(<domain|file>... | --all) [--verify-renewal]",
			summary: "Manage hand-written host files (only a marker comment is added)", write: true, run: adoptCmd},
		command{path: "adopt release", args: "<domain>", summary: "Stop managing an adopted host file (its content is kept)",
			write: true, run: adoptRelease},
	)
}

type certbotJSON struct {
	Name          string   `json:"name"`
	RenewalFile   string   `json:"renewal_file"`
	Authenticator string   `json:"authenticator,omitempty"`
	Installer     string   `json:"installer,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

func toCertbotJSON(certPath string) *certbotJSON {
	if certPath == "" {
		return nil
	}
	l, ok := certificates.CertbotLineage(certPath)
	if !ok {
		return nil
	}
	return &certbotJSON{l.Name, l.RenewalFile, l.Authenticator, l.Installer, l.Notes()}
}

type adoptedJSON struct {
	File         string       `json:"file"`
	Name         string       `json:"name"`
	Mode         string       `json:"mode"`
	Certificate  string       `json:"certificate,omitempty"`
	Certbot      *certbotJSON `json:"certbot,omitempty"`
	HTTPBlock    bool         `json:"separate_http_block,omitempty"`
	Kept         []string     `json:"kept,omitempty"`
	Custom       []string     `json:"custom_locations,omitempty"`
	Notes        []string     `json:"notes"`
	RenewalCheck string       `json:"renewal_check,omitempty"`
}

type skippedJSON struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

func adoptCmd(e *env, args []string) error {
	fs := flag.NewFlagSet("adopt", flag.ContinueOnError)
	all := fs.Bool("all", false, "adopt every hand-written host")
	verify := fs.Bool("verify-renewal", false, "afterwards, run `certbot renew --dry-run` for certificates certbot renews")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if *all == (len(pos) > 0) {
		return usageErr("give the hosts to adopt, or --all")
	}

	list, err := e.svc.Manager.ListHosts()
	if err != nil {
		return err
	}
	var files []string
	var skipped []skippedJSON
	if *all {
		for _, h := range list {
			if !h.Managed && h.AdoptError == "" {
				files = append(files, h.FileName)
			}
		}
	}
	for _, name := range pos {
		h, err := e.findHost(name)
		switch {
		case err != nil:
			skipped = append(skipped, skippedJSON{name, err.Error()})
		case h.Adopted != nil || h.AdoptError != "":
			skipped = append(skipped, skippedJSON{h.FileName, "already adopted"})
		case h.Managed:
			skipped = append(skipped, skippedJSON{h.FileName, "created by GoNix, already managed"})
		default:
			files = append(files, h.FileName)
		}
	}

	ctx := context.Background()
	var adopted []adoptedJSON
	for _, file := range files {
		rep, err := e.svc.HostService.AdoptHost(ctx, file)
		if err != nil {
			skipped = append(skipped, skippedJSON{file, err.Error()})
			continue
		}
		h := rep.Host
		a := adoptedJSON{File: file, Name: h.ServerName, Mode: string(h.Mode), HTTPBlock: rep.HTTPBlock, Kept: rep.Kept, Custom: rep.Custom,
			Notes: hosts.AdoptionNotes(rep)}
		if h.SSL.Enabled {
			a.Certificate = h.SSL.CertFile
			a.Certbot = toCertbotJSON(h.SSL.CertFile)
		}
		if a.Certbot != nil && *verify && !e.DryRun {
			a.RenewalCheck = certbotDryRun(ctx, a.Certbot.Name)
			a.Notes = append(a.Notes, "renewal check: "+a.RenewalCheck)
		}
		adopted = append(adopted, a)
	}

	if e.DryRun {
		for _, a := range adopted {
			for _, n := range a.Notes {
				e.plan.RecordNote(a.File + ": " + n)
			}
		}
		for _, s := range skipped {
			e.plan.RecordNote("skip " + s.File + ": " + s.Reason)
		}
		return e.printPlan()
	}
	if len(adopted) == 0 && len(skipped) > 0 {
		var reasons []string
		for _, s := range skipped {
			reasons = append(reasons, s.File+": "+s.Reason)
		}
		return fmt.Errorf("nothing was adopted:\n  %s", strings.Join(reasons, "\n  "))
	}
	if adopted == nil {
		adopted = []adoptedJSON{}
	}
	return e.show(map[string]any{"ok": true, "adopted": adopted, "skipped": skipped}, func(w io.Writer) {
		if len(adopted) == 0 {
			fmt.Fprintln(w, "No hand-written hosts to adopt.")
		}
		for _, a := range adopted {
			fmt.Fprintf(w, "Adopted %s (%s, %s)\n", a.Name, filepath.Join(e.svc.Manager.SitesAvailable, a.File), a.Mode)
			for _, n := range a.Notes {
				fmt.Fprintln(w, "  "+n)
			}
		}
		for _, s := range skipped {
			fmt.Fprintf(w, "Skipped %s: %s\n", s.File, s.Reason)
		}
	})
}

// certbotDryRun runs certbot's renewal against the staging server, which
// changes nothing, and summarizes the outcome.
func certbotDryRun(ctx context.Context, name string) string {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "certbot", "renew", "--dry-run", "--cert-name", name, "--non-interactive")
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		return fmt.Sprintf("FAILED (%v): %s", err, strings.Join(lines[max(0, len(lines)-5):], " | "))
	}
	return "passed (certbot renew --dry-run)"
}

func adoptRelease(e *env, args []string) error {
	pos, err := parseFlags(flag.NewFlagSet("adopt release", flag.ContinueOnError), args)
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
	if h.Adopted == nil && h.AdoptError == "" {
		return fmt.Errorf("%s is not an adopted host", h.ServerName)
	}
	if err := e.svc.HostService.ReleaseHost(context.Background(), h.FileName); err != nil {
		return err
	}
	return e.done("Host " + h.ServerName + " is no longer managed by GoNix; its file was kept as it is.")
}

// adoptedState describes how GoNix manages a listed host, for host lists.
func adoptedState(h nginx.ListedHost) string {
	switch {
	case h.AdoptError != "":
		return "adopted, unreadable"
	case h.Adopted != nil:
		return "adopted"
	}
	return ""
}
