// Package realip manages the http-level real client IP configuration. When
// Nginx sits behind a CDN or load balancer, $remote_addr is the proxy's
// address, so IP access lists, rate limits and logs all see the wrong
// client. ngx_http_realip_module fixes that by trusting a header, but only
// for requests coming from the listed proxies.
package realip

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

// FileName is the http-level file GoNix writes the settings to.
const FileName = "gonix-realip.conf"

// Headers a trusted proxy may carry the client address in.
var Headers = []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP"}

// CloudflareRanges are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips/), used until refreshed online.
var CloudflareRanges = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
}

// CloudflareURLs publish the current ranges, one CIDR per line.
var CloudflareURLs = []string{"https://www.cloudflare.com/ips-v4", "https://www.cloudflare.com/ips-v6"}

// Settings is the real client IP configuration.
type Settings struct {
	Cloudflare bool     // Proxies are Cloudflare's ranges, Header is CF-Connecting-IP
	Header     string   // one of Headers
	Proxies    []string // trusted proxy addresses or CIDR ranges
}

// Validate checks that the settings render to safe Nginx syntax.
func (s Settings) Validate() error {
	valid := false
	for _, h := range Headers {
		if s.Header == h {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("header %q is not one of %s", s.Header, strings.Join(Headers, ", "))
	}
	if len(s.Proxies) == 0 {
		return fmt.Errorf("at least one trusted proxy address or range is required")
	}
	for _, p := range s.Proxies {
		if err := iplist.ValidateEntry(p); err != nil {
			return fmt.Errorf("trusted proxy: %w", err)
		}
	}
	return nil
}

// Render produces the file content.
func Render(s Settings) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	source := "custom"
	if s.Cloudflare {
		source = "cloudflare"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Real client IP (%s)\n", nginx.ManagedMarker, source)
	fmt.Fprintf(&b, "# Requests from these trusted proxies take the client address from %s.\n", s.Header)
	for _, p := range s.Proxies {
		fmt.Fprintf(&b, "set_real_ip_from %s;\n", p)
	}
	fmt.Fprintf(&b, "real_ip_header %s;\n", s.Header)
	b.WriteString("real_ip_recursive on;\n")
	return b.String(), nil
}

var (
	reSource = regexp.MustCompile(`Real client IP \((\w+)\)`)
	reFrom   = regexp.MustCompile(`(?m)^\s*set_real_ip_from\s+([^;\s]+)\s*;`)
	reHeader = regexp.MustCompile(`(?m)^\s*real_ip_header\s+([^;\s]+)\s*;`)
)

// Parse reconstructs Settings from content produced by Render.
func Parse(content string) Settings {
	var s Settings
	if m := reSource.FindStringSubmatch(content); m != nil {
		s.Cloudflare = m[1] == "cloudflare"
	}
	for _, m := range reFrom.FindAllStringSubmatch(content, -1) {
		s.Proxies = append(s.Proxies, m[1])
	}
	if m := reHeader.FindStringSubmatch(content); m != nil {
		s.Header = m[1]
	}
	return s
}

// FetchCloudflareRanges downloads Cloudflare's current edge ranges. Every
// line is validated, so a broken or hijacked response cannot inject
// configuration.
func FetchCloudflareRanges(ctx context.Context, client *http.Client) ([]string, error) {
	var ranges []string
	for _, url := range CloudflareURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", url, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("fetching %s: %s", url, resp.Status)
		}
		lines, err := readRanges(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", url, err)
		}
		ranges = append(ranges, lines...)
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("Cloudflare returned no IP ranges")
	}
	return ranges, nil
}

func readRanges(r io.Reader) ([]string, error) {
	var ranges []string
	sc := bufio.NewScanner(io.LimitReader(r, 64<<10))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if err := iplist.ValidateEntry(line); err != nil {
			return nil, err
		}
		ranges = append(ranges, line)
	}
	return ranges, sc.Err()
}

// Service reads and applies the settings file.
type Service struct {
	Dir    string // http-level snippets directory, e.g. /etc/nginx/conf.d
	Writer *snippet.Writer
}

// Path is the settings file path.
func (s *Service) Path() string { return filepath.Join(s.Dir, FileName) }

// Get returns the current settings, and false when real client IP handling
// is not enabled.
func (s *Service) Get() (Settings, bool, error) {
	data, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, false, nil
		}
		return Settings{}, false, fmt.Errorf("reading %s: %w", s.Path(), err)
	}
	return Parse(string(data)), true, nil
}

// Save writes the settings and applies them safely.
func (s *Service) Save(ctx context.Context, settings Settings) error {
	content, err := Render(settings)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.Dir, err)
	}
	return s.Writer.Write(ctx, "real_ip_updated", "real-ip", s.Path(), content)
}

// Disable removes the settings file.
func (s *Service) Disable(ctx context.Context) error {
	return s.Writer.Remove(ctx, "real_ip_disabled", "real-ip", s.Path())
}
