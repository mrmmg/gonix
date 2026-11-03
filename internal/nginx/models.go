// Package nginx contains the domain model, configuration renderer, filesystem
// operations and validator used to manage Nginx virtual hosts.
package nginx

import (
	"fmt"
	"net/url"
	"strings"
)

// HostMode describes the high level purpose of a virtual host.
type HostMode string

const (
	ModeReverseProxy HostMode = "reverse_proxy"
	ModeStatic       HostMode = "static"
	ModeRedirect     HostMode = "redirect"
)

// RedirectCodes are the status codes a redirect host may answer with.
var RedirectCodes = []int{301, 302, 307, 308}

// RedirectConfig holds the settings of a host in ModeRedirect, which
// answers every request with a redirect to another URL.
type RedirectConfig struct {
	Target       string // absolute http(s) URL, e.g. https://example.com
	Code         int    // one of RedirectCodes
	PreservePath bool   // append the original path and query ($request_uri)
}

// ReturnURL is the URL rendered into the `return` directive.
func (r RedirectConfig) ReturnURL() string {
	if !r.PreservePath {
		return r.Target
	}
	return strings.TrimSuffix(r.Target, "/") + "$request_uri"
}

// NormalizeRedirectTarget turns a bare domain such as "example.com" into
// "https://example.com"; anything with a scheme is returned trimmed but
// otherwise unchanged.
func NormalizeRedirectTarget(target string) string {
	target = strings.TrimSpace(target)
	if target != "" && !strings.Contains(target, "://") {
		target = "https://" + target
	}
	return target
}

// ValidateRedirectTarget checks that target is an absolute http(s) URL that
// can be rendered into a `return` directive without breaking its syntax.
func ValidateRedirectTarget(target string) error {
	if target == "" {
		return fmt.Errorf("redirect target must not be empty")
	}
	if strings.ContainsAny(target, " \t\n;{}\"'$#\\") {
		return fmt.Errorf("redirect target %q contains characters that are not allowed", target)
	}
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("redirect target %q is not a valid URL: %w", target, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("redirect target %q must start with http:// or https://", target)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("redirect target %q has no host", target)
	}
	return nil
}

func validRedirectCode(code int) bool {
	for _, c := range RedirectCodes {
		if c == code {
			return true
		}
	}
	return false
}

// KeyType identifies the private key algorithm used by a certificate.
type KeyType string

const (
	KeyTypeUnknown KeyType = "unknown"
	KeyTypeRSA     KeyType = "rsa"
	KeyTypeECDSA   KeyType = "ecdsa"
)

// SSLConfig holds the TLS settings applied to a host.
type SSLConfig struct {
	Enabled    bool
	CertFile   string
	KeyFile    string
	KeyType    KeyType
	HTTP2      bool
	HTTP3      bool // also listen for QUIC on ListenSSL (UDP); see BuildInfo.SupportsHTTP3
	ForceHTTPS bool // redirect plain HTTP requests to HTTPS
}

// ProxyConfig groups the reverse proxy specific settings of a location.
type ProxyConfig struct {
	UpstreamScheme   string // "http" or "https"
	UpstreamHost     string
	UpstreamPort     int
	HTTPVersion      string // "1.0", "1.1"
	ConnectTimeout   string // e.g. "60s"
	ReadTimeout      string
	SendTimeout      string
	WebSocket        bool
	ForwardHost      bool
	ForwardFor       bool
	ForwardProto     bool
	MaxBodySize      string // client_max_body_size, e.g. "10m"
	BufferingEnabled bool
}

// UpstreamURL renders the proxy_pass target, e.g. http://127.0.0.1:8080.
func (p ProxyConfig) UpstreamURL() string {
	return fmt.Sprintf("%s://%s:%d", p.UpstreamScheme, p.UpstreamHost, p.UpstreamPort)
}

// Location represents a single `location` block inside a server block.
type Location struct {
	Path           string
	IsCustom       bool   // true when CustomConfig should be used verbatim
	CustomConfig   string // raw user supplied configuration for the location
	Proxy          *ProxyConfig
	AccessListName string // empty when no access list is applied
	IPListName     string // IP access list included by this location, empty if none
	RateLimit      string // rate limit included by this location, empty if none
}

// Host is the internal representation of a single Nginx virtual host,
// independent of how it is ultimately rendered to disk.
type Host struct {
	ServerName     string // primary domain, also used as the config file name
	ServerAlias    []string
	Mode           HostMode
	Listen         int            // plain HTTP port, normally 80
	ListenSSL      int            // HTTPS port, normally 443
	Root           string         // used when Mode == ModeStatic
	Redirect       RedirectConfig // used when Mode == ModeRedirect
	SSL            SSLConfig
	Locations      []Location
	AccessLog      bool
	ErrorLog       bool
	AccessListName string // access list applied at server level, empty if none
	IPListName     string // IP access list included at server level, empty if none
	RateLimit      string // rate limit included at server level, empty if none

	// SatisfyAny renders `satisfy any;`, so a client passes when it matches
	// the IP list OR authenticates, instead of having to do both.
	SatisfyAny bool

	// ErrorPagesSnippet names the error pages snippet included by this
	// host (see internal/errorpages), empty if none.
	ErrorPagesSnippet string

	// Presets names the shared preset snippets (see internal/presets)
	// included at server level, e.g. "security-headers".
	Presets []string

	// Managed is false when the configuration file was not generated by
	// gonix (e.g. hand written). Such hosts are shown read-only
	// unless the user explicitly opts into management.
	Managed bool

	// Enabled reflects whether a symlink exists in sites-enabled. This is
	// computed at discovery time, not stored in the config file.
	Enabled bool
}

// Validate performs basic structural validation of the host model before it
// is rendered to an Nginx configuration file. It does not invoke `nginx -t`;
// that is the job of the Validator.
func (h Host) Validate() error {
	if h.ServerName == "" {
		return fmt.Errorf("server name must not be empty")
	}
	if h.Mode == ModeReverseProxy && len(h.Locations) == 0 {
		return fmt.Errorf("reverse proxy host %q must have at least one location", h.ServerName)
	}
	if h.Mode == ModeStatic && h.Root == "" {
		return fmt.Errorf("static host %q must have a root directory", h.ServerName)
	}
	if h.Mode == ModeRedirect {
		if err := ValidateRedirectTarget(h.Redirect.Target); err != nil {
			return fmt.Errorf("redirect host %q: %w", h.ServerName, err)
		}
		if !validRedirectCode(h.Redirect.Code) {
			return fmt.Errorf("redirect host %q: status code %d is not one of %v", h.ServerName, h.Redirect.Code, RedirectCodes)
		}
	}
	for _, loc := range h.Locations {
		if loc.Path == "" {
			return fmt.Errorf("location in host %q has an empty path", h.ServerName)
		}
		if !loc.IsCustom && loc.Proxy == nil {
			return fmt.Errorf("location %q in host %q is not custom but has no proxy configuration", loc.Path, h.ServerName)
		}
	}
	if h.IPListName != "" && !ValidSnippetName(h.IPListName) {
		return fmt.Errorf("host %q references an invalid IP access list name %q", h.ServerName, h.IPListName)
	}
	if h.RateLimit != "" && !ValidSnippetName(h.RateLimit) {
		return fmt.Errorf("host %q references an invalid rate limit name %q", h.ServerName, h.RateLimit)
	}
	for _, loc := range h.Locations {
		if loc.IPListName != "" && !ValidSnippetName(loc.IPListName) {
			return fmt.Errorf("location %q in host %q references an invalid IP access list name %q", loc.Path, h.ServerName, loc.IPListName)
		}
		if loc.RateLimit != "" && !ValidSnippetName(loc.RateLimit) {
			return fmt.Errorf("location %q in host %q references an invalid rate limit name %q", loc.Path, h.ServerName, loc.RateLimit)
		}
	}
	for _, p := range h.Presets {
		if !ValidSnippetName(p) {
			return fmt.Errorf("host %q references an invalid preset name %q", h.ServerName, p)
		}
	}
	if h.SSL.HTTP3 && !h.SSL.Enabled {
		return fmt.Errorf("host %q enables HTTP/3 without SSL", h.ServerName)
	}
	if h.ErrorPagesSnippet != "" && !ValidSnippetName(h.ErrorPagesSnippet) {
		return fmt.Errorf("host %q references an invalid error pages snippet name %q", h.ServerName, h.ErrorPagesSnippet)
	}
	if h.SSL.Enabled {
		if h.SSL.CertFile == "" || h.SSL.KeyFile == "" {
			return fmt.Errorf("host %q has SSL enabled but is missing certificate paths", h.ServerName)
		}
	}
	return nil
}
