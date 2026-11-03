// Package npm imports hosts from Nginx Proxy Manager. Instead of reading
// NPM's database (which would need an SQLite driver), it parses the Nginx
// files NPM generates from its fixed templates under <data>/nginx:
// proxy_host, redirection_host and stream. Settings GoNix has an equivalent
// for are mapped onto it; anything else is reported, never silently lost.
package npm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/presets"
	"github.com/mrmmg/gonix/internal/streams"
)

// Item is one NPM host or stream and what GoNix would make of it.
type Item struct {
	Source   string // NPM file
	Host     *nginx.Host
	Stream   *streams.Proxy
	CertFrom string // NPM certificate directory to copy, if any
	// AccessFile is NPM's htpasswd file for the host's access list, copied
	// as access list AccessName.
	AccessFile, AccessName string
	// Allow/deny rules of the access list, imported as IP list IPListName.
	IPAllow, IPDeny []string
	IPListName      string
	Warnings        []string
	Skip            string // reason the item cannot be imported
}

// Name is a short label for reports.
func (it Item) Name() string {
	switch {
	case it.Host != nil:
		return it.Host.ServerName
	case it.Stream != nil:
		return "stream " + it.Stream.Name
	}
	return filepath.Base(it.Source)
}

var (
	reServerName = regexp.MustCompile(`(?m)^\s*server_name\s+([^;]+);`)
	reSetVar     = regexp.MustCompile(`(?m)^\s*set\s+\$(forward_scheme|server|port)\s+"?([^";\s]+)"?\s*;`)
	reCert       = regexp.MustCompile(`(?m)^\s*ssl_certificate\s+(\S+)/fullchain\.pem\s*;`)
	reHTTP2      = regexp.MustCompile(`(?m)^\s*http2\s+on\s*;`)
	reListenSSL  = regexp.MustCompile(`(?m)^\s*listen\s+\d+\s+ssl`)
	reReturn     = regexp.MustCompile(`(?m)^\s*return\s+(\d{3})\s+(\S+?)\s*;`)
	reLocation   = regexp.MustCompile(`(?m)^\s*location\s+(\S+)\s*\{`)
	reProxyPass  = regexp.MustCompile(`(?m)^\s*proxy_pass\s+(\S+?)\s*;`)
	reAuthFile   = regexp.MustCompile(`(?m)^\s*auth_basic_user_file\s+\S*/access/(\d+)\s*;`)
	reAllow      = regexp.MustCompile(`(?m)^\s*allow\s+([^;\s]+)\s*;`)
	reDeny       = regexp.MustCompile(`(?m)^\s*deny\s+([^;\s]+)\s*;`)
	reStreamPort = regexp.MustCompile(`(?m)^\s*listen\s+(\d+)(\s+udp)?\s*;`)
)

// known lines of NPM's templates that GoNix either maps or makes obsolete;
// any other directive at server level came from "Advanced" and is reported.
var knownServerLine = regexp.MustCompile(`^(set \$|listen |#|server_name |http2 |include conf\.d/include/|include /data/nginx/custom/|ssl_certificate|add_header Strict-Transport-Security|proxy_set_header (Upgrade|Connection)|proxy_http_version|access_log |error_log |server \{|\}|location |$)`)

// Scan reads NPM's generated configuration under dataDir (NPM's /data).
// letsencryptDir is NPM's /etc/letsencrypt, used to find certificates.
func Scan(dataDir, letsencryptDir string) ([]Item, error) {
	var items []Item
	for _, kind := range []string{"proxy_host", "redirection_host", "stream"} {
		files, err := filepath.Glob(filepath.Join(dataDir, "nginx", kind, "*.conf"))
		if err != nil {
			return nil, err
		}
		sort.Slice(files, func(i, j int) bool { return confID(files[i]) < confID(files[j]) })
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			content := string(data)
			if !strings.Contains(content, "server {") {
				continue // disabled in NPM: the file holds only its header
			}
			switch kind {
			case "stream":
				items = append(items, parseStream(f, content)...)
			default:
				items = append(items, parseHost(f, kind, content, dataDir, letsencryptDir))
			}
		}
	}
	if len(items) == 0 {
		if _, err := os.Stat(filepath.Join(dataDir, "nginx")); err != nil {
			return nil, fmt.Errorf("%s does not look like Nginx Proxy Manager's data directory (no nginx/ inside)", dataDir)
		}
	}
	return items, nil
}

func confID(path string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(path), ".conf"))
	return n
}

func parseHost(file, kind, content, dataDir, letsencryptDir string) Item {
	it := Item{Source: file}
	m := reServerName.FindStringSubmatch(content)
	if m == nil {
		it.Skip = "no server_name"
		return it
	}
	names := strings.Fields(m[1])
	var valid []string
	for _, n := range names {
		if err := hosts.ValidateDomain(n); err != nil {
			it.Warnings = append(it.Warnings, fmt.Sprintf("domain %s skipped (%v)", n, err))
			continue
		}
		valid = append(valid, n)
	}
	if len(valid) == 0 {
		it.Skip = "no domain GoNix can manage (wildcards are not supported)"
		return it
	}
	h := &nginx.Host{ServerName: valid[0], ServerAlias: valid[1:], Listen: 80, ListenSSL: 443, AccessLog: true, ErrorLog: true}
	it.Host = h

	serverLevel, locations := splitLocations(content)
	if kind == "redirection_host" {
		ret := reReturn.FindStringSubmatch(content)
		if ret == nil {
			it.Skip = "redirect target not found"
			return it
		}
		code, _ := strconv.Atoi(ret[1])
		target := ret[2]
		preserve := strings.HasSuffix(target, "$request_uri")
		target = strings.TrimSuffix(target, "$request_uri")
		target = strings.Replace(target, "$scheme://", "https://", 1)
		h.Mode = nginx.ModeRedirect
		h.Redirect = nginx.RedirectConfig{Target: target, Code: code, PreservePath: preserve}
		if err := nginx.ValidateRedirectTarget(target); err != nil {
			it.Skip = err.Error()
			return it
		}
	} else {
		vars := map[string]string{}
		for _, v := range reSetVar.FindAllStringSubmatch(content, -1) {
			vars[v[1]] = v[2]
		}
		port, err := strconv.Atoi(vars["port"])
		if err != nil || vars["server"] == "" {
			it.Skip = "forward host/port not found"
			return it
		}
		scheme := vars["forward_scheme"]
		if scheme != "https" {
			scheme = "http"
		}
		h.Mode = nginx.ModeReverseProxy
		websocket := strings.Contains(serverLevel, "proxy_set_header Upgrade $http_upgrade")
		h.Locations = []nginx.Location{{Path: "/", Proxy: &nginx.ProxyConfig{
			UpstreamScheme: scheme, UpstreamHost: vars["server"], UpstreamPort: port, HTTPVersion: "1.1",
			WebSocket: websocket, ForwardHost: true, ForwardFor: true, ForwardProto: true, BufferingEnabled: true,
		}}}
		for _, loc := range locations {
			if loc.path == "/" {
				it.parseAccess(loc.body, dataDir)
				continue
			}
			pp := reProxyPass.FindStringSubmatch(loc.body)
			if pp == nil || strings.Contains(pp[1], "$") {
				it.Warnings = append(it.Warnings, "custom location "+loc.path+" not imported (no plain proxy_pass)")
				continue
			}
			scheme, host, port, ok := splitURL(pp[1])
			if !ok {
				it.Warnings = append(it.Warnings, "custom location "+loc.path+" not imported (proxy_pass with a path)")
				continue
			}
			h.Locations = append(h.Locations, nginx.Location{Path: loc.path, Proxy: &nginx.ProxyConfig{
				UpstreamScheme: scheme, UpstreamHost: host, UpstreamPort: port, HTTPVersion: "1.1",
				ForwardHost: true, ForwardFor: true, ForwardProto: true, BufferingEnabled: true,
			}})
		}
	}

	if reListenSSL.MatchString(content) {
		if c := reCert.FindStringSubmatch(content); c != nil {
			src := c[1]
			if strings.HasPrefix(src, "/etc/letsencrypt/") {
				src = filepath.Join(letsencryptDir, strings.TrimPrefix(src, "/etc/letsencrypt/"))
			} else if strings.HasPrefix(src, "/data/") {
				src = filepath.Join(dataDir, strings.TrimPrefix(src, "/data/"))
			}
			if _, err := os.Stat(filepath.Join(src, "fullchain.pem")); err == nil {
				it.CertFrom = src
				h.SSL = nginx.SSLConfig{Enabled: true, HTTP2: reHTTP2.MatchString(content),
					ForceHTTPS: strings.Contains(content, "include conf.d/include/force-ssl.conf;")}
			} else {
				it.Warnings = append(it.Warnings, "certificate "+c[1]+" not found; imported without SSL (add it later)")
			}
		}
	}
	if strings.Contains(content, "include conf.d/include/block-exploits.conf;") {
		h.Presets = append(h.Presets, "block-sensitive-files", "block-bad-bots")
	}
	if h.SSL.Enabled && strings.Contains(content, "Strict-Transport-Security") {
		h.Presets = append(h.Presets, "hsts")
	}
	if strings.Contains(content, "include conf.d/include/assets.conf;") && h.Mode != nginx.ModeRedirect {
		h.Presets = append(h.Presets, "static-cache")
	}
	for _, line := range strings.Split(serverLevel, "\n") {
		line = strings.TrimSpace(line)
		if !knownServerLine.MatchString(line) {
			it.Warnings = append(it.Warnings, "advanced configuration not imported: "+line)
		}
	}
	return it
}

// parseAccess picks up NPM's access list on location /.
func (it *Item) parseAccess(body, dataDir string) {
	id := ""
	if m := reAuthFile.FindStringSubmatch(body); m != nil {
		id = m[1]
		it.AccessFile = filepath.Join(dataDir, "access", id)
		it.AccessName = "npm-" + id
		if _, err := os.Stat(it.AccessFile); err != nil {
			it.Warnings = append(it.Warnings, "access list file "+it.AccessFile+" not found; Basic Auth not imported")
			it.AccessFile, it.AccessName = "", ""
		}
	}
	for _, m := range reAllow.FindAllStringSubmatch(body, -1) {
		if m[1] != "all" {
			it.IPAllow = append(it.IPAllow, m[1])
		}
	}
	for _, m := range reDeny.FindAllStringSubmatch(body, -1) {
		if m[1] != "all" {
			it.IPDeny = append(it.IPDeny, m[1])
		}
	}
	if len(it.IPAllow)+len(it.IPDeny) > 0 {
		if id == "" {
			id = strings.ReplaceAll(it.Host.ServerName, ".", "_")
		}
		it.IPListName = "npm-" + id
		if len(it.IPAllow) > 0 && len(it.IPDeny) > 0 {
			it.Warnings = append(it.Warnings, "access list mixes allow and deny entries; only the allow entries were imported")
			it.IPDeny = nil
		}
	}
	if it.AccessName != "" && it.IPListName != "" && strings.Contains(body, "satisfy any") {
		it.Host.SatisfyAny = true
	}
}

type location struct{ path, body string }

// splitLocations returns the server-level lines and each location block.
func splitLocations(content string) (string, []location) {
	var server strings.Builder
	var locs []location
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		m := reLocation.FindStringSubmatch(lines[i])
		if m == nil {
			server.WriteString(lines[i] + "\n")
			continue
		}
		depth := strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
		var body strings.Builder
		for i++; i < len(lines) && depth > 0; i++ {
			depth += strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
			if depth > 0 {
				body.WriteString(lines[i] + "\n")
			}
		}
		i--
		locs = append(locs, location{m[1], body.String()})
	}
	return server.String(), locs
}

func splitURL(u string) (scheme, host string, port int, ok bool) {
	scheme = "http"
	rest := u
	if s, r, found := strings.Cut(u, "://"); found {
		scheme, rest = s, r
	}
	if scheme != "http" && scheme != "https" || strings.Contains(rest, "/") {
		return "", "", 0, false
	}
	host = rest
	port = 80
	if scheme == "https" {
		port = 443
	}
	if i := strings.LastIndexByte(rest, ':'); i > 0 {
		p, err := strconv.Atoi(rest[i+1:])
		if err != nil {
			return "", "", 0, false
		}
		host, port = rest[:i], p
	}
	return scheme, host, port, host != ""
}

func parseStream(file, content string) []Item {
	var out []Item
	blocks := strings.Split(content, "server {")[1:]
	for _, b := range blocks {
		m := reStreamPort.FindStringSubmatch(b)
		pp := reProxyPass.FindStringSubmatch(b)
		it := Item{Source: file}
		if m == nil || pp == nil {
			it.Skip = "listen port or backend not found"
			out = append(out, it)
			continue
		}
		port, _ := strconv.Atoi(m[1])
		udp := m[2] != ""
		proto := "tcp"
		if udp {
			proto = "udp"
		}
		it.Stream = &streams.Proxy{Name: fmt.Sprintf("npm-%d-%s", port, proto), Port: port, UDP: udp, Upstream: pp[1],
			ConnectTimeout: "10s", Timeout: "10m", Enabled: true}
		if err := it.Stream.Validate(); err != nil {
			it.Skip = err.Error()
		}
		out = append(out, it)
	}
	return out
}

// Copier copies files GoNix needs from the NPM data (certificates, htpasswd
// files) into GoNix's directories.
type Copier func(src, dst string, mode os.FileMode) error

// Target is where an import puts things.
type Target struct {
	CertsDir string
	// Exists reports whether a host with this server name already exists.
	Exists func(serverName string) bool
}

// Prepare resolves certificate paths into the GoNix certificates directory
// and marks items whose host already exists.
func Prepare(items []Item, t Target) {
	for i := range items {
		it := &items[i]
		if it.Host == nil || it.Skip != "" {
			continue
		}
		if t.Exists != nil && t.Exists(it.Host.ServerName) {
			it.Skip = "a host with this name already exists in GoNix"
			continue
		}
		if it.CertFrom != "" {
			dir := filepath.Join(t.CertsDir, it.Host.ServerName)
			it.Host.SSL.CertFile = filepath.Join(dir, "fullchain.pem")
			it.Host.SSL.KeyFile = filepath.Join(dir, "privkey.pem")
		}
		if it.AccessName != "" {
			it.Host.AccessListName = it.AccessName
		}
		if it.IPListName != "" {
			it.Host.IPListName = it.IPListName
		}
		it.Host.Presets = presets.KeepApplicable(*it.Host)
		it.Host.Presets = presets.Sort(it.Host.Presets)
	}
}

// Runner performs the import steps; the CLI and TUI wire it to the real
// services.
type Runner struct {
	CopyFile     Copier
	CertsDir     string
	AccessDir    string
	CreateIPList func(ctx context.Context, name string, allow bool, entries []string) error
	EnsurePreset func(ctx context.Context, names []string) error
	CreateHost   func(ctx context.Context, h nginx.Host) error
	SaveStream   func(ctx context.Context, p streams.Proxy) error
}

// Result reports what an import did.
type Result struct {
	Imported []string
	Skipped  []string
	Warnings []string
}

// Run imports every item that is not skipped. A failing item is reported
// and the others continue.
func (r *Runner) Run(ctx context.Context, items []Item) Result {
	var res Result
	for _, it := range items {
		name := it.Name()
		for _, w := range it.Warnings {
			res.Warnings = append(res.Warnings, name+": "+w)
		}
		if it.Skip != "" {
			res.Skipped = append(res.Skipped, name+": "+it.Skip)
			continue
		}
		if err := r.importItem(ctx, it); err != nil {
			res.Skipped = append(res.Skipped, name+": "+err.Error())
			continue
		}
		res.Imported = append(res.Imported, name)
	}
	return res
}

func (r *Runner) importItem(ctx context.Context, it Item) error {
	if it.Stream != nil {
		return r.SaveStream(ctx, *it.Stream)
	}
	if it.CertFrom != "" {
		for _, f := range []string{"fullchain.pem", "privkey.pem"} {
			mode := os.FileMode(0o644)
			if f == "privkey.pem" {
				mode = 0o600
			}
			dst := filepath.Join(r.CertsDir, it.Host.ServerName, f)
			if _, err := os.Stat(dst); err == nil {
				continue // keep a certificate GoNix already has
			}
			if err := r.CopyFile(filepath.Join(it.CertFrom, f), dst, mode); err != nil {
				return fmt.Errorf("copying certificate: %w", err)
			}
		}
	}
	if it.AccessFile != "" {
		dst := filepath.Join(r.AccessDir, it.AccessName+".htpasswd")
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			if err := r.CopyFile(it.AccessFile, dst, 0o640); err != nil {
				return fmt.Errorf("copying access list: %w", err)
			}
		}
	}
	if it.IPListName != "" {
		allow, entries := true, it.IPAllow
		if len(entries) == 0 {
			allow, entries = false, it.IPDeny
		}
		if err := r.CreateIPList(ctx, it.IPListName, allow, entries); err != nil {
			return err
		}
	}
	if len(it.Host.Presets) > 0 {
		if err := r.EnsurePreset(ctx, it.Host.Presets); err != nil {
			return err
		}
	}
	return r.CreateHost(ctx, *it.Host)
}

// CopyFile is the default Copier.
func CopyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}
