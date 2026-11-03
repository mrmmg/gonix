// Package presets provides one-click host settings (security headers, HSTS,
// compression, ...). Each preset is a shared snippet GoNix owns and keeps up
// to date; hosts opt in by including it at server level, so their own
// configuration files stay short and readable.
package presets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

// Preset is a named, shared server-level snippet.
type Preset struct {
	Name        string
	Title       string
	Description string
	// SSLOnly presets only make sense on HTTPS hosts (e.g. HSTS).
	SSLOnly bool
	// ForRedirect presets also apply to redirect hosts, whose server-level
	// `return` still passes through header filters.
	ForRedirect bool
	// ProxyOnly presets only apply to reverse proxy hosts.
	ProxyOnly bool
	// Server is included by hosts at server level.
	Server string
	// HTTP, when set, is written to the http-level snippets directory,
	// e.g. a map the server snippet relies on. CacheDirPlaceholder is
	// replaced with Service.CacheDir.
	HTTP string
}

// All lists the available presets in display order.
var All = []Preset{
	{
		Name:        "security-headers",
		Title:       "Security headers",
		Description: "X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy (replacing the upstream's own)",
		Server: `# Common security headers. The upstream's own copies are hidden so each
# header is sent exactly once. A location with its own add_header does not
# inherit these.
proxy_hide_header X-Frame-Options;
proxy_hide_header X-Content-Type-Options;
proxy_hide_header Referrer-Policy;
proxy_hide_header Permissions-Policy;
add_header X-Frame-Options "SAMEORIGIN" always;
add_header X-Content-Type-Options "nosniff" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;
`,
	},
	{
		Name:        "hsts",
		Title:       "HSTS",
		Description: "browsers only use HTTPS for this domain for one year (subdomains are not affected)",
		SSLOnly:     true,
		ForRedirect: true,
		Server: `# HTTP Strict Transport Security: browsers that saw this header refuse
# plain HTTP for this host for max-age seconds. includeSubDomains and
# preload are deliberately left out; add them by hand once every
# subdomain serves HTTPS, since they are hard to undo.
proxy_hide_header Strict-Transport-Security;
add_header Strict-Transport-Security "max-age=31536000" always;
`,
	},
	{
		Name:        "gzip",
		Title:       "Compression (gzip)",
		Description: "compress text, JSON, JavaScript, CSS, SVG and fonts",
		Server: `# gzip compression for text-based responses. text/html is always
# compressed once gzip is on.
gzip on;
gzip_vary on;
gzip_proxied any;
gzip_comp_level 5;
gzip_min_length 256;
gzip_types text/plain text/css text/xml text/javascript application/javascript application/json application/xml application/rss+xml application/atom+xml image/svg+xml font/ttf font/otf application/vnd.ms-fontobject;
`,
	},
	{
		Name:        "static-cache",
		Title:       "Static asset caching",
		Description: "browsers cache CSS, JavaScript, images, fonts and video for 30 days",
		Server: `# Sets Expires/Cache-Control by response type. The map lives in the
# http-level file gonix-preset-static-cache.conf.
expires $gonix_static_expires;
`,
		HTTP: `map $sent_http_content_type $gonix_static_expires {
    default                 off;
    ~^text/css              30d;
    ~javascript             30d;
    ~^image/                30d;
    ~^font/                 30d;
    ~^application/font      30d;
    ~^video/                30d;
}
`,
	},
	{
		Name:        "proxy-cache",
		Title:       "Proxy cache",
		Description: "cache responses the backend marks cacheable; requests with cookies or Authorization always reach the backend",
		ProxyOnly:   true,
		Server: `# Only responses the backend allows (Cache-Control max-age, Expires) are
# cached; responses with Set-Cookie or Cache-Control private/no-store
# never are. Requests carrying cookies or credentials skip the cache, so
# one visitor's personalized page is never served to another.
proxy_cache gonix_cache;
proxy_cache_bypass $http_cookie $http_authorization;
proxy_no_cache $http_cookie $http_authorization;
proxy_cache_use_stale error timeout updating http_500 http_502 http_503 http_504;
proxy_cache_background_update on;
proxy_cache_lock on;
`,
		HTTP: `proxy_cache_path ` + CacheDirPlaceholder + ` levels=1:2 keys_zone=gonix_cache:50m max_size=1g inactive=60m use_temp_path=off;
`,
	},
	{
		Name:        "block-sensitive-files",
		Title:       "Block sensitive files",
		Description: "404 for dotfiles (.git, .env, ...) and backup/dump files (.bak, .sql, ...), except /.well-known/",
		Server: `# Hidden files and directories such as .git or .env, except
# /.well-known/ (used by ACME and others).
location ~ /\.(?!well-known/) {
    return 404;
}

# Backup, editor and dump files left next to served content.
location ~* \.(?:bak|old|orig|save|swp|sql|log)$ {
    return 404;
}
`,
	},
	{
		Name:        "block-bad-bots",
		Title:       "Block vulnerability scanners",
		Description: "403 for common scanner user agents (sqlmap, nikto, nuclei, ...)",
		Server: `# Common vulnerability scanners identify themselves in User-Agent.
# Determined attackers change it, but this cheaply stops automated noise.
if ($http_user_agent ~* "(sqlmap|nikto|nmap|masscan|zgrab|nuclei|wpscan|dirbuster|gobuster|ffuf|acunetix|netsparker|jorgee)") {
    return 403;
}
`,
	},
}

// Get returns the preset called name.
func Get(name string) (Preset, bool) {
	for _, p := range All {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// Applicable returns the presets that make sense for h.
func Applicable(h nginx.Host) []Preset {
	var out []Preset
	for _, p := range All {
		if p.SSLOnly && !h.SSL.Enabled {
			continue
		}
		if h.Mode == nginx.ModeRedirect && !p.ForRedirect {
			continue
		}
		if p.ProxyOnly && h.Mode != nginx.ModeReverseProxy {
			continue
		}
		out = append(out, p)
	}
	return out
}

// KeepApplicable drops the presets h can no longer use, e.g. HSTS after
// SSL was turned off.
func KeepApplicable(h nginx.Host) []string {
	ok := map[string]bool{}
	for _, p := range Applicable(h) {
		ok[p.Name] = true
	}
	var out []string
	for _, n := range h.Presets {
		if ok[n] {
			out = append(out, n)
		}
	}
	return out
}

// Sort orders names like All and drops unknown or duplicate ones, so host
// files list presets consistently.
func Sort(names []string) []string {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var out []string
	for _, p := range All {
		if want[p.Name] {
			out = append(out, p.Name)
		}
	}
	return out
}

func header(p Preset) string {
	return fmt.Sprintf("%s Preset: %s\n# Rewritten by GoNix whenever a host enables it; edits here are overwritten.\n", nginx.ManagedMarker, p.Name)
}

// Service writes preset snippets.
type Service struct {
	SnippetsDir string // server-level snippets, included by hosts
	HTTPDir     string // http-level snippets, e.g. /etc/nginx/conf.d
	CacheDir    string // proxy cache directory, e.g. /var/cache/nginx/gonix
	Writer      *snippet.Writer
}

// CacheDirPlaceholder in a preset's HTTP content is replaced with
// Service.CacheDir.
const CacheDirPlaceholder = "{{CACHE_DIR}}"

// ServerPath and HTTPPath return a preset's snippet file paths.
func (s *Service) ServerPath(name string) string {
	return filepath.Join(s.SnippetsDir, nginx.PresetFileName(name))
}

func (s *Service) HTTPPath(name string) string {
	return filepath.Join(s.HTTPDir, nginx.PresetFileName(name))
}

// Ensure writes (or refreshes) the snippets of the named presets, so hosts
// can include them. Files already up to date are left alone, and everything
// is validated with a single `nginx -t`.
func (s *Service) Ensure(ctx context.Context, names []string) error {
	var files []snippet.File
	for _, name := range names {
		p, ok := Get(name)
		if !ok {
			return fmt.Errorf("unknown preset %q", name)
		}
		files = appendIfChanged(files, s.ServerPath(name), header(p)+p.Server)
		if p.HTTP != "" {
			if err := os.MkdirAll(s.HTTPDir, 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", s.HTTPDir, err)
			}
			http := p.HTTP
			if strings.Contains(http, CacheDirPlaceholder) {
				if err := validateCacheDir(s.CacheDir); err != nil {
					return err
				}
				http = strings.ReplaceAll(http, CacheDirPlaceholder, s.CacheDir)
			}
			files = appendIfChanged(files, s.HTTPPath(name), header(p)+http)
		}
	}
	if len(files) == 0 {
		return nil
	}
	return s.Writer.Apply(ctx, "presets_updated", strings.Join(names, ","), files...)
}

func validateCacheDir(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) == "/" || strings.ContainsAny(dir, " \t\n;{}\"'$#") {
		return fmt.Errorf("proxy cache directory %q must be an absolute path without spaces or any of ; { } \" ' $ #", dir)
	}
	return nil
}

func appendIfChanged(files []snippet.File, path, content string) []snippet.File {
	if data, err := os.ReadFile(path); err == nil && string(data) == content {
		return files
	}
	return append(files, snippet.File{Path: path, Content: &content})
}
