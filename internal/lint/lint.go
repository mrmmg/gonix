// Package lint looks for configuration problems `nginx -t` does not report:
// conflicts between hosts, references to files that no longer exist,
// certificates about to expire, settings that silently have no effect, and
// leftovers nothing uses any more.
package lint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/health"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/presets"
	"github.com/mrmmg/gonix/internal/upstreams"
)

// Severity ranks findings.
type Severity int

const (
	Error   Severity = iota // breaks something now
	Warning                 // likely a mistake, or about to break
	Info                    // harmless, worth cleaning up
)

func (s Severity) String() string {
	return [...]string{"ERROR", "WARN", "INFO"}[s]
}

// Finding is one problem, about a host or a shared file.
type Finding struct {
	Severity Severity
	Target   string
	Message  string
}

// Checker holds where everything lives.
type Checker struct {
	Manager       *nginx.Manager
	ConfigDir     string // e.g. /etc/nginx, for the nginx.conf include check
	HTTPDir       string // http-level snippets (conf.d)
	SnippetsDir   string // server-level snippets
	AccessListDir string
	// CheckUpstreams dials every reverse-proxy upstream.
	CheckUpstreams bool
	// ExpiryWarning is how long before expiry a certificate is reported.
	ExpiryWarning time.Duration
}

var reCustomReturn = regexp.MustCompile(`(?m)^\s*return\s`)
var reCustomAddHeader = regexp.MustCompile(`(?m)^\s*add_header\s`)

// Run performs every check and returns the findings, most severe first.
func (c *Checker) Run(ctx context.Context) ([]Finding, error) {
	hostList, err := c.Manager.ListHosts()
	if err != nil {
		return nil, err
	}
	var out []Finding
	add := func(sev Severity, target, format string, args ...any) {
		out = append(out, Finding{sev, target, fmt.Sprintf(format, args...)})
	}

	// Server names claimed by more than one enabled host: Nginx answers
	// with whichever it loaded first and warns about the rest.
	owners := map[string][]string{}
	for _, h := range hostList {
		if h.BrokenSymlink {
			add(Error, h.FileName, "sites-enabled/%s points to a file that does not exist", h.FileName)
		}
		if !h.Enabled {
			continue
		}
		for _, name := range append([]string{h.ServerName}, h.ServerAlias...) {
			key := strings.ToLower(name)
			owners[key] = append(owners[key], h.FileName)
		}
	}
	for name, files := range owners {
		if len(files) > 1 {
			add(Error, name, "server name is used by %d enabled hosts (%s); only the first one is served",
				len(files), strings.Join(files, ", "))
		}
	}

	used := map[string]bool{} // snippet/list file paths referenced by hosts
	maintenance := map[string]bool{}
	var upstreams []string
	upstreamHosts := map[string][]string{}
	for _, h := range hostList {
		if !h.Managed {
			continue
		}
		c.checkHost(h.Host, add, used)
		if h.Maintenance {
			maintenance[nginx.MaintenanceFileName(h.ServerName)] = true
		}
		if c.CheckUpstreams && h.Enabled {
			for _, u := range c.upstreamAddrs(h.Host) {
				if upstreamHosts[u] == nil {
					upstreams = append(upstreams, u)
				}
				upstreamHosts[u] = append(upstreamHosts[u], h.ServerName)
			}
		}
	}

	if len(upstreams) > 0 {
		for addr, err := range health.Check(ctx, upstreams, 2*time.Second) {
			add(Warning, strings.Join(upstreamHosts[addr], ", "), "upstream %s is not accepting connections (%v); visitors get 502", addr, err)
		}
	}

	c.checkSharedFiles(add, used, maintenance)

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity < out[j].Severity
		}
		return out[i].Target < out[j].Target
	})
	return out, nil
}

func (c *Checker) checkHost(h nginx.Host, add func(Severity, string, string, ...any), used map[string]bool) {
	name := h.ServerName
	need := func(path, what string) {
		used[path] = true
		if _, err := os.Stat(path); err != nil {
			add(Error, name, "%s %s is missing; nginx -t fails until it is restored or no longer used", what, path)
		}
	}
	snippet := func(fileName func(string) string, n string) string {
		return filepath.Join(c.SnippetsDir, fileName(n))
	}

	if h.AccessListName != "" {
		need(filepath.Join(c.AccessListDir, h.AccessListName+".htpasswd"), "access list")
	}
	if h.IPListName != "" {
		need(snippet(nginx.IPListFileName, h.IPListName), "IP access list")
	}
	if h.RateLimit != "" {
		need(snippet(nginx.RateLimitFileName, h.RateLimit), "rate limit")
		need(filepath.Join(c.HTTPDir, nginx.RateLimitFileName(h.RateLimit)), "rate limit zone")
	}
	if h.ErrorPagesSnippet != "" {
		need(snippet(nginx.SnippetFileName, h.ErrorPagesSnippet), "error pages snippet")
	}
	if h.Maintenance {
		need(filepath.Join(c.HTTPDir, nginx.MaintenanceFileName(h.ServerName)), "maintenance file")
	}
	headerPresets := false
	for _, p := range h.Presets {
		need(snippet(nginx.PresetFileName, p), "preset")
		if pr, ok := presets.Get(p); ok {
			if pr.HTTP != "" {
				need(filepath.Join(c.HTTPDir, nginx.PresetFileName(p)), "preset http-level file")
			}
			if strings.Contains(pr.Server, "add_header") {
				headerPresets = true
			}
		}
	}
	proxyCache := false
	for _, p := range h.Presets {
		proxyCache = proxyCache || p == "proxy-cache"
	}
	for _, loc := range h.Locations {
		if proxyCache && loc.Proxy != nil && !loc.Proxy.BufferingEnabled {
			add(Warning, name, "location %s has proxy buffering off, so the proxy cache preset never caches it", loc.Path)
		}
		if loc.AccessListName != "" {
			need(filepath.Join(c.AccessListDir, loc.AccessListName+".htpasswd"), "access list")
		}
		if loc.IPListName != "" {
			need(snippet(nginx.IPListFileName, loc.IPListName), "IP access list")
		}
		if loc.RateLimit != "" {
			need(snippet(nginx.RateLimitFileName, loc.RateLimit), "rate limit")
			need(filepath.Join(c.HTTPDir, nginx.RateLimitFileName(loc.RateLimit)), "rate limit zone")
		}
		if loc.Proxy != nil && loc.Proxy.UpstreamGroup != "" {
			need(filepath.Join(c.HTTPDir, nginx.UpstreamFileName(loc.Proxy.UpstreamGroup)), "upstream group")
		}
		if !loc.IsCustom {
			continue
		}
		if (h.IPListName != "" || h.AccessListName != "") && reCustomReturn.MatchString(loc.CustomConfig) {
			add(Warning, name, "custom location %s uses return, which answers before IP rules and Basic Auth are checked; they do not protect it", loc.Path)
		}
		if headerPresets && reCustomAddHeader.MatchString(loc.CustomConfig) {
			add(Warning, name, "custom location %s has its own add_header, so it does not inherit the headers of this host's presets", loc.Path)
		}
	}

	if h.SSL.Enabled {
		if h.Mode != nginx.ModeRedirect && !h.SSL.ForceHTTPS {
			add(Warning, name, "SSL is enabled but plain HTTP is not redirected to HTTPS")
		}
		c.checkCertificate(h, add)
	}
}

func (c *Checker) checkCertificate(h nginx.Host, add func(Severity, string, string, ...any)) {
	if _, err := os.Stat(h.SSL.CertFile); err != nil {
		add(Error, h.ServerName, "certificate %s is missing", h.SSL.CertFile)
		return
	}
	cert := certificates.Parse(filepath.Base(filepath.Dir(h.SSL.CertFile)), filepath.Dir(h.SSL.CertFile), h.SSL.CertFile, h.SSL.KeyFile)
	if cert.ParseError != "" {
		add(Error, h.ServerName, "certificate %s cannot be read: %s", h.SSL.CertFile, cert.ParseError)
		return
	}
	left := time.Until(cert.NotAfter)
	switch {
	case left <= 0:
		add(Error, h.ServerName, "certificate expired on %s", cert.ExpiresAtExact())
	case left < c.expiryWarning():
		add(Warning, h.ServerName, "certificate %s (%s)", cert.RemainingHuman(), cert.ExpiresAtExact())
	}
	if !certCovers(cert, h.ServerName) {
		add(Warning, h.ServerName, "certificate %s does not list %s among its names", h.SSL.CertFile, h.ServerName)
	}
}

// upstreamAddrs returns the backends h proxies to, including the servers of
// its upstream groups.
func (c *Checker) upstreamAddrs(h nginx.Host) []string {
	addrs := health.Upstreams(h)
	for _, loc := range h.Locations {
		if loc.Proxy == nil || loc.Proxy.UpstreamGroup == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.HTTPDir, nginx.UpstreamFileName(loc.Proxy.UpstreamGroup)))
		if err != nil {
			continue
		}
		for _, s := range upstreams.Parse(loc.Proxy.UpstreamGroup, string(data)).Servers {
			if !s.Backup {
				addrs = append(addrs, s.Address)
			}
		}
	}
	return addrs
}

func (c *Checker) expiryWarning() time.Duration {
	if c.ExpiryWarning > 0 {
		return c.ExpiryWarning
	}
	return 14 * 24 * time.Hour
}

// certCovers reports whether name matches one of the certificate's SANs,
// including one-level wildcards.
func certCovers(cert certificates.Certificate, name string) bool {
	name = strings.ToLower(name)
	for _, san := range cert.SANs {
		san = strings.ToLower(san)
		if san == name {
			return true
		}
		if rest, ok := strings.CutPrefix(san, "*."); ok {
			if i := strings.IndexByte(name, '.'); i > 0 && name[i+1:] == rest {
				return true
			}
		}
	}
	return false
}

// checkSharedFiles reports GoNix files nothing uses, and http-level files
// that have no effect because nginx.conf does not include their directory.
func (c *Checker) checkSharedFiles(add func(Severity, string, string, ...any), used, maintenance map[string]bool) {
	httpFiles := 0
	if entries, err := os.ReadDir(c.HTTPDir); err == nil {
		for _, e := range entries {
			n := e.Name()
			if !strings.HasPrefix(n, "gonix-") {
				continue
			}
			httpFiles++
			if strings.HasPrefix(n, nginx.UpstreamFilePrefix) && !used[filepath.Join(c.HTTPDir, n)] {
				add(Info, filepath.Join(c.HTTPDir, n), "upstream group is not used by any GoNix host")
			}
			if strings.HasPrefix(n, nginx.MaintenanceFilePrefix) && !maintenance[n] {
				add(Info, filepath.Join(c.HTTPDir, n), "maintenance file of a host that is no longer in maintenance mode")
			}
		}
	}
	if httpFiles > 0 {
		if ok, err := nginx.IncludesDir(c.ConfigDir, c.HTTPDir); err == nil && !ok {
			add(Error, filepath.Join(c.ConfigDir, "nginx.conf"),
				"does not include %s, so GoNix's http-level settings (real IP, rate limit zones, maintenance) have no effect; add `include %s/*.conf;` to the http block",
				c.HTTPDir, c.HTTPDir)
		}
	}

	if entries, err := os.ReadDir(c.SnippetsDir); err == nil {
		for _, e := range entries {
			path := filepath.Join(c.SnippetsDir, e.Name())
			kind := ""
			switch {
			case strings.HasPrefix(e.Name(), nginx.IPListFilePrefix):
				kind = "IP access list"
			case strings.HasPrefix(e.Name(), nginx.RateLimitFilePrefix):
				kind = "rate limit"
			case strings.HasPrefix(e.Name(), nginx.SnippetFilePrefix):
				kind = "error pages snippet"
			}
			if kind != "" && !used[path] {
				add(Info, path, "%s is not used by any GoNix host", kind)
			}
		}
	}
	if entries, err := os.ReadDir(c.AccessListDir); err == nil {
		for _, e := range entries {
			path := filepath.Join(c.AccessListDir, e.Name())
			if strings.HasSuffix(e.Name(), ".htpasswd") && !used[path] {
				add(Info, path, "access list is not used by any GoNix host")
			}
		}
	}
}
