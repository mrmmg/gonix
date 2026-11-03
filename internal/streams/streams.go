// Package streams manages TCP/UDP stream proxies (databases, SSH, game
// servers, DNS, ...) through Nginx's stream module. Each proxy is one file
// in a directory that a stream { } block includes. Nginx allows only one
// stream block, so GoNix creates it only when none exists; otherwise the
// admin adds a single include line to their own.
package streams

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
	"github.com/mrmmg/gonix/internal/upstreams"
)

// WrapperFileName is the main-level file holding GoNix's stream block.
const WrapperFileName = "gonix-stream.conf"

const disabledSuffix = ".disabled"

// Proxy forwards a TCP or UDP port to a backend.
type Proxy struct {
	Name           string
	Port           int
	UDP            bool
	Upstream       string // host:port
	ConnectTimeout string // e.g. "10s"
	Timeout        string // idle timeout between reads/writes, e.g. "10m"
	IPListName     string // optional IP access list (allow/deny work in stream too)
	Enabled        bool
}

var reDuration = regexp.MustCompile(`^\d+(ms|s|m|h)?$`)

// Validate checks that the proxy renders to safe Nginx syntax.
func (p Proxy) Validate() error {
	if !nginx.ValidSnippetName(p.Name) {
		return fmt.Errorf("name %q may only contain letters, digits, '-' and '_'", p.Name)
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("listen port must be between 1 and 65535")
	}
	if err := upstreams.ValidateAddress(p.Upstream); err != nil {
		return fmt.Errorf("backend: %w", err)
	}
	for _, d := range []string{p.ConnectTimeout, p.Timeout} {
		if d != "" && !reDuration.MatchString(d) {
			return fmt.Errorf("timeout %q must be a duration such as 10s or 10m", d)
		}
	}
	if p.IPListName != "" && !nginx.ValidSnippetName(p.IPListName) {
		return fmt.Errorf("invalid IP access list name %q", p.IPListName)
	}
	return nil
}

// Protocol is "tcp" or "udp".
func (p Proxy) Protocol() string {
	if p.UDP {
		return "udp"
	}
	return "tcp"
}

// Render produces the proxy's server block. snippetsDir locates IP lists.
func Render(p Proxy, snippetsDir string) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Stream proxy: %s\n", nginx.ManagedMarker, p.Name)
	b.WriteString("server {\n")
	if p.UDP {
		fmt.Fprintf(&b, "    listen %d udp;\n", p.Port)
	} else {
		fmt.Fprintf(&b, "    listen %d;\n", p.Port)
	}
	fmt.Fprintf(&b, "    proxy_pass %s;\n", p.Upstream)
	if p.ConnectTimeout != "" {
		fmt.Fprintf(&b, "    proxy_connect_timeout %s;\n", p.ConnectTimeout)
	}
	if p.Timeout != "" {
		fmt.Fprintf(&b, "    proxy_timeout %s;\n", p.Timeout)
	}
	if p.IPListName != "" {
		fmt.Fprintf(&b, "    include %s;\n", filepath.Join(snippetsDir, nginx.IPListFileName(p.IPListName)))
	}
	b.WriteString("}\n")
	return b.String(), nil
}

var (
	reListen    = regexp.MustCompile(`(?m)^\s*listen\s+(\d+)(\s+udp)?\s*;`)
	reProxyPass = regexp.MustCompile(`(?m)^\s*proxy_pass\s+([^;\s]+)\s*;`)
	reConnTO    = regexp.MustCompile(`(?m)^\s*proxy_connect_timeout\s+([^;\s]+)\s*;`)
	reTimeout   = regexp.MustCompile(`(?m)^\s*proxy_timeout\s+([^;\s]+)\s*;`)
	reIPList    = regexp.MustCompile(`(?m)^\s*include\s+\S*/` + nginx.IPListFilePrefix + `([A-Za-z0-9_-]+)\.conf\s*;`)
)

// Parse reconstructs a Proxy from content produced by Render.
func Parse(name, content string) Proxy {
	p := Proxy{Name: name}
	if m := reListen.FindStringSubmatch(content); m != nil {
		p.Port, _ = strconv.Atoi(m[1])
		p.UDP = m[2] != ""
	}
	if m := reProxyPass.FindStringSubmatch(content); m != nil {
		p.Upstream = m[1]
	}
	if m := reConnTO.FindStringSubmatch(content); m != nil {
		p.ConnectTimeout = m[1]
	}
	if m := reTimeout.FindStringSubmatch(content); m != nil {
		p.Timeout = m[1]
	}
	if m := reIPList.FindStringSubmatch(content); m != nil {
		p.IPListName = m[1]
	}
	return p
}

// RenderWrapper produces the stream block that includes dir.
func RenderWrapper(dir string) string {
	return fmt.Sprintf("%s Stream (TCP/UDP) proxies.\n# Nginx allows a single stream block: if you add your own, move this\n# include into it and delete this file.\nstream {\n    include %s/*.conf;\n}\n",
		nginx.ManagedMarker, dir)
}

// Service manages stream proxies on disk.
type Service struct {
	Dir            string // one file per proxy
	MainIncludeDir string // where the wrapper stream block is placed
	ConfigDir      string // for reading nginx.conf
	SnippetsDir    string // IP access lists
	Writer         *snippet.Writer
}

func (s *Service) path(name string, enabled bool) string {
	p := filepath.Join(s.Dir, name+".conf")
	if !enabled {
		p += disabledSuffix
	}
	return p
}

// WrapperPath is the main-level file holding GoNix's stream block.
func (s *Service) WrapperPath() string { return filepath.Join(s.MainIncludeDir, WrapperFileName) }

var reStreamBlock = regexp.MustCompile(`(?m)^\s*stream\s*\{`)

// Ready reports whether stream proxies take effect: GoNix's wrapper is
// installed, or nginx.conf itself includes Dir. When not ready, the
// returned text explains what to do.
func (s *Service) Ready() (bool, string) {
	if _, err := os.Stat(s.WrapperPath()); err == nil {
		return true, ""
	}
	if ok, _ := nginx.IncludesDir(s.ConfigDir, s.Dir); ok {
		return true, ""
	}
	if s.hasStreamBlock() {
		return false, fmt.Sprintf("Your Nginx configuration already has a stream { } block. Add this line inside it:\n\n    include %s/*.conf;", s.Dir)
	}
	if ok, _ := nginx.IncludesDir(s.ConfigDir, s.MainIncludeDir); !ok {
		return false, fmt.Sprintf("Add this to the top level of %s (outside http { }):\n\n    stream {\n        include %s/*.conf;\n    }",
			filepath.Join(s.ConfigDir, "nginx.conf"), s.Dir)
	}
	return false, "Stream support is not set up yet; GoNix can set it up for you."
}

// CanSetUp reports whether SetUp can create the stream block automatically.
func (s *Service) CanSetUp() bool {
	ok, _ := nginx.IncludesDir(s.ConfigDir, s.MainIncludeDir)
	return ok && !s.hasStreamBlock()
}

func (s *Service) hasStreamBlock() bool {
	files := []string{filepath.Join(s.ConfigDir, "nginx.conf")}
	if entries, err := os.ReadDir(s.MainIncludeDir); err == nil {
		for _, e := range entries {
			if e.Name() != WrapperFileName {
				files = append(files, filepath.Join(s.MainIncludeDir, e.Name()))
			}
		}
	}
	for _, f := range files {
		if data, err := os.ReadFile(f); err == nil && reStreamBlock.MatchString(string(data)) {
			return true
		}
	}
	return false
}

// SetUp writes the wrapper stream block. `nginx -t` rejects it (and it is
// removed again) when the stream module is not available.
func (s *Service) SetUp(ctx context.Context) error {
	if !s.CanSetUp() {
		_, why := s.Ready()
		return fmt.Errorf("stream support cannot be set up automatically. %s", why)
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.Dir, err)
	}
	if err := s.Writer.Write(ctx, "streams_setup", "stream", s.WrapperPath(), RenderWrapper(s.Dir)); err != nil {
		return fmt.Errorf("%w\n\nIf Nginx reports an unknown \"stream\" directive, install its stream module (Debian/Ubuntu: libnginx-mod-stream, Arch: nginx-mod-stream)", err)
	}
	return nil
}

// List returns every proxy, enabled or not, sorted by name.
func (s *Service) List() ([]Proxy, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", s.Dir, err)
	}
	var out []Proxy
	for _, e := range entries {
		name, enabled := e.Name(), true
		if n, ok := strings.CutSuffix(name, disabledSuffix); ok {
			name, enabled = n, false
		}
		name, ok := strings.CutSuffix(name, ".conf")
		if e.IsDir() || !ok || !nginx.ValidSnippetName(name) {
			continue
		}
		if p, err := s.Get(name); err == nil && p.Enabled == enabled {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get loads a single proxy.
func (s *Service) Get(name string) (Proxy, error) {
	for _, enabled := range []bool{true, false} {
		if data, err := os.ReadFile(s.path(name, enabled)); err == nil {
			p := Parse(name, string(data))
			p.Enabled = enabled
			return p, nil
		}
	}
	return Proxy{}, fmt.Errorf("stream proxy %q does not exist", name)
}

// Exists reports whether a proxy called name exists.
func (s *Service) Exists(name string) bool {
	_, err := s.Get(name)
	return err == nil
}

// Save creates or updates a proxy, refusing a port another proxy already
// listens on with the same protocol.
func (s *Service) Save(ctx context.Context, p Proxy, create bool) error {
	if create && s.Exists(p.Name) {
		return fmt.Errorf("a stream proxy named %q already exists", p.Name)
	}
	if !create && !s.Exists(p.Name) {
		return fmt.Errorf("stream proxy %q does not exist", p.Name)
	}
	others, err := s.List()
	if err != nil {
		return err
	}
	for _, o := range others {
		if o.Name != p.Name && o.Port == p.Port && o.UDP == p.UDP {
			return fmt.Errorf("stream proxy %q already listens on %s port %d", o.Name, p.Protocol(), p.Port)
		}
	}
	content, err := Render(p, s.SnippetsDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.Dir, err)
	}
	action := "stream_updated"
	if create {
		action = "stream_created"
	}
	// Writing the enabled or disabled file also removes the other one, so a
	// proxy never exists twice.
	return s.Writer.Apply(ctx, action, p.Name,
		snippet.File{Path: s.path(p.Name, p.Enabled), Content: &content},
		snippet.File{Path: s.path(p.Name, !p.Enabled)},
	)
}

// SetEnabled enables or disables a proxy without changing its settings.
func (s *Service) SetEnabled(ctx context.Context, name string, enabled bool) error {
	p, err := s.Get(name)
	if err != nil {
		return err
	}
	p.Enabled = enabled
	return s.Save(ctx, p, false)
}

// UsingIPList returns "stream <name>" for every proxy including the IP
// access list called list.
func (s *Service) UsingIPList(list string) []string {
	proxies, _ := s.List()
	var out []string
	for _, p := range proxies {
		if p.IPListName == list {
			out = append(out, "stream "+p.Name)
		}
	}
	return out
}

// Delete removes a proxy.
func (s *Service) Delete(ctx context.Context, name string) error {
	return s.Writer.Apply(ctx, "stream_deleted", name,
		snippet.File{Path: s.path(name, true)},
		snippet.File{Path: s.path(name, false)},
	)
}
