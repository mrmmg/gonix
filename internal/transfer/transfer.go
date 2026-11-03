// Package transfer exports everything GoNix manages into one archive and
// imports it on another server: managed hosts and which of them are
// enabled, snippets, http-level files, access lists and stream proxies.
// Paths inside the files are rewritten when the target server keeps them
// elsewhere. HTML error pages and certificates are not included; they are
// the admin's own content and are copied separately.
package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

// Archive sections, each mapped to a local directory by Dirs.
const (
	SectionSites       = "sites"
	SectionSnippets    = "snippets"
	SectionHTTP        = "http"
	SectionAccessLists = "accesslists"
	SectionStreams     = "streams"
	SectionMain        = "main"
)

const manifestName = "manifest.json"

// Dirs locates each section on a server.
type Dirs struct {
	SitesAvailable string
	SitesEnabled   string
	Snippets       string
	HTTP           string
	AccessLists    string
	Streams        string
	Main           string // main-level include directory (stream wrapper)
}

func (d Dirs) section(name string) string {
	switch name {
	case SectionSites:
		return d.SitesAvailable
	case SectionSnippets:
		return d.Snippets
	case SectionHTTP:
		return d.HTTP
	case SectionAccessLists:
		return d.AccessLists
	case SectionStreams:
		return d.Streams
	case SectionMain:
		return d.Main
	}
	return ""
}

// Manifest describes an archive.
type Manifest struct {
	Version  int       `json:"version"`
	Created  time.Time `json:"created"`
	Hostname string    `json:"hostname"`
	Dirs     Dirs      `json:"dirs"`
	Enabled  []string  `json:"enabled"` // enabled host file names
}

// selectFile decides which files of each section belong to GoNix.
func selectFile(section, name string) bool {
	switch section {
	case SectionSnippets, SectionHTTP:
		return strings.HasPrefix(name, "gonix-")
	case SectionAccessLists:
		return strings.HasSuffix(name, ".htpasswd")
	case SectionStreams:
		return strings.HasSuffix(name, ".conf") || strings.HasSuffix(name, ".conf.disabled")
	case SectionMain:
		return name == "gonix-stream.conf"
	}
	return false
}

// Export writes an archive of everything GoNix manages to w, and returns
// its manifest.
func Export(w io.Writer, dirs Dirs, mgr *nginx.Manager) (Manifest, error) {
	host, _ := os.Hostname()
	m := Manifest{Version: 1, Created: time.Now().UTC(), Hostname: host, Dirs: dirs}

	type entry struct{ name, path string }
	var entries []entry
	hosts, err := mgr.ListHosts()
	if err != nil {
		return m, err
	}
	for _, h := range hosts {
		if !h.Managed {
			continue
		}
		entries = append(entries, entry{path.Join(SectionSites, h.FileName), filepath.Join(dirs.SitesAvailable, h.FileName)})
		if h.Enabled {
			m.Enabled = append(m.Enabled, h.FileName)
		}
	}
	for _, section := range []string{SectionSnippets, SectionHTTP, SectionAccessLists, SectionStreams, SectionMain} {
		dir := dirs.section(section)
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.Type().IsRegular() && selectFile(section, f.Name()) {
				entries = append(entries, entry{path.Join(section, f.Name()), filepath.Join(dir, f.Name())})
			}
		}
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	manifest, _ := json.MarshalIndent(m, "", "  ")
	if err := writeEntry(tw, manifestName, manifest, 0o644); err != nil {
		return m, err
	}
	for _, e := range entries {
		data, err := os.ReadFile(e.path)
		if err != nil {
			return m, fmt.Errorf("reading %s: %w", e.path, err)
		}
		mode := int64(0o644)
		if strings.HasPrefix(e.name, SectionAccessLists+"/") {
			mode = 0o600
		}
		if err := writeEntry(tw, e.name, data, mode); err != nil {
			return m, err
		}
	}
	if err := tw.Close(); err != nil {
		return m, err
	}
	return m, gz.Close()
}

func writeEntry(tw *tar.Writer, name string, data []byte, mode int64) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), ModTime: time.Now(), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// File is one file an import would write.
type File struct {
	Section string
	Name    string
	Target  string // local path
	Content string
	Exists  bool // a file is already there
	Same    bool // ... with identical content
}

// Plan is what an import would do.
type Plan struct {
	Manifest Manifest
	Files    []File
	Enable   []string // host file names to enable
}

// Conflicts lists files that exist locally with different content.
func (p Plan) Conflicts() []string {
	var out []string
	for _, f := range p.Files {
		if f.Exists && !f.Same {
			out = append(out, f.Target)
		}
	}
	return out
}

// Read parses an archive into a Plan for dirs, rewriting the source
// server's directories to the local ones inside every file.
func Read(r io.Reader, dirs Dirs) (Plan, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return Plan{}, fmt.Errorf("not a GoNix export (gzip): %w", err)
	}
	tr := tar.NewReader(gz)
	var plan Plan
	contents := map[string]string{}
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Plan{}, fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Size > 16<<20 {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, 16<<20))
		if err != nil {
			return Plan{}, err
		}
		if hdr.Name == manifestName {
			if err := json.Unmarshal(data, &plan.Manifest); err != nil {
				return Plan{}, fmt.Errorf("reading manifest: %w", err)
			}
			continue
		}
		contents[hdr.Name] = string(data)
		names = append(names, hdr.Name)
	}
	if plan.Manifest.Version != 1 {
		return Plan{}, fmt.Errorf("not a GoNix export (missing or unsupported manifest)")
	}

	src := plan.Manifest.Dirs
	replacer := strings.NewReplacer(rewritePairs(src, dirs)...)
	sort.Strings(names)
	for _, name := range names {
		section, file, ok := strings.Cut(name, "/")
		if !ok || strings.Contains(file, "/") || file == ".." || file == "." || !selectFileOrSite(section, file) {
			continue // unknown or unsafe entry
		}
		target := filepath.Join(dirs.section(section), file)
		f := File{Section: section, Name: file, Target: target, Content: replacer.Replace(contents[name])}
		if data, err := os.ReadFile(target); err == nil {
			f.Exists, f.Same = true, string(data) == f.Content
		}
		plan.Files = append(plan.Files, f)
	}
	for _, e := range plan.Manifest.Enabled {
		if contents[path.Join(SectionSites, e)] != "" {
			plan.Enable = append(plan.Enable, e)
		}
	}
	return plan, nil
}

func selectFileOrSite(section, name string) bool {
	if section == SectionSites {
		return name != "" && !strings.HasPrefix(name, ".")
	}
	return selectFile(section, name)
}

// rewritePairs maps every source directory to its local counterpart,
// longest first so nested directories are replaced correctly.
func rewritePairs(src, dst Dirs) []string {
	type pair struct{ from, to string }
	var pairs []pair
	for _, s := range []string{SectionSites, SectionSnippets, SectionHTTP, SectionAccessLists, SectionStreams, SectionMain} {
		from, to := src.section(s), dst.section(s)
		if from != "" && to != "" && from != to {
			pairs = append(pairs, pair{from + "/", to + "/"})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return len(pairs[i].from) > len(pairs[j].from) })
	var out []string
	for _, p := range pairs {
		out = append(out, p.from, p.to)
	}
	return out
}

// Importer applies a Plan.
type Importer struct {
	Dirs    Dirs
	Manager *nginx.Manager
	Writer  *snippet.Writer
	// AfterWrite runs once the files are in place, e.g. to give access
	// list files the Nginx worker group.
	AfterWrite func() error
}

// Apply writes every file of plan (all or nothing, under one nginx -t),
// then enables the hosts that were enabled on the source server. Existing
// files with different content are only replaced when overwrite is set.
func (im *Importer) Apply(ctx context.Context, plan Plan, overwrite bool) error {
	if c := plan.Conflicts(); len(c) > 0 && !overwrite {
		return fmt.Errorf("these files already exist with different content (import with overwrite to replace them):\n  %s", strings.Join(c, "\n  "))
	}
	var files []snippet.File
	for _, f := range plan.Files {
		if f.Same {
			continue
		}
		content := f.Content
		mode := os.FileMode(0o644)
		if f.Section == SectionAccessLists {
			mode = 0o640
		}
		files = append(files, snippet.File{Path: f.Target, Content: &content, Mode: mode})
	}
	if len(files) > 0 {
		if err := im.Writer.Apply(ctx, "config_imported", plan.Manifest.Hostname, files...); err != nil {
			return err
		}
	}
	if im.AfterWrite != nil {
		if err := im.AfterWrite(); err != nil {
			return err
		}
	}

	var enabled []string
	for _, name := range plan.Enable {
		if !im.Manager.IsEnabled(name) {
			if err := im.Manager.Enable(name); err != nil {
				return err
			}
			enabled = append(enabled, name)
		}
	}
	if len(enabled) == 0 {
		return nil
	}
	// Enabling hosts is validated as one more step; if it fails, the
	// imported files stay but the hosts are left disabled.
	if err := im.Writer.Apply(ctx, "config_import_enabled", strings.Join(enabled, ",")); err != nil {
		for _, name := range enabled {
			_ = im.Manager.Disable(name)
		}
		return fmt.Errorf("files imported, but enabling the hosts failed, so they were left disabled: %w", err)
	}
	return nil
}
