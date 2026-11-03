// Package upstreams implements named upstream groups: several backend
// servers behind one name, with a load balancing method and Nginx's passive
// health checks (a server that fails max_fails times is skipped for
// fail_timeout). Reverse-proxy locations can target a group instead of a
// single host:port.
package upstreams

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

// Load balancing methods.
const (
	RoundRobin = "round_robin"
	LeastConn  = "least_conn"
	IPHash     = "ip_hash"
)

// Methods lists the supported load balancing methods.
var Methods = []string{RoundRobin, LeastConn, IPHash}

// Server is one backend of a group.
type Server struct {
	Address string // host:port
	Weight  int    // 0 or 1 = default weight
	Backup  bool   // only used when every other server is down
}

// Group is a named upstream group.
type Group struct {
	Name        string
	Method      string
	Servers     []Server
	MaxFails    int    // failed attempts before a server is skipped
	FailTimeout string // how long a failed server is skipped, e.g. "10s"
}

var reDuration = regexp.MustCompile(`^\d+(ms|s|m|h)?$`)

// Validate checks that the group renders to safe Nginx syntax.
func (g Group) Validate() error {
	if !nginx.ValidSnippetName(g.Name) {
		return fmt.Errorf("group name %q may only contain letters, digits, '-' and '_'", g.Name)
	}
	if !validMethod(g.Method) {
		return fmt.Errorf("unknown load balancing method %q", g.Method)
	}
	if len(g.Servers) == 0 {
		return fmt.Errorf("group %q needs at least one server", g.Name)
	}
	primary := 0
	for _, s := range g.Servers {
		if err := ValidateAddress(s.Address); err != nil {
			return err
		}
		if s.Weight < 0 || s.Weight > 1000 {
			return fmt.Errorf("weight of %s must be between 1 and 1000", s.Address)
		}
		if s.Backup && g.Method == IPHash {
			return fmt.Errorf("backup servers cannot be used with ip_hash")
		}
		if !s.Backup {
			primary++
		}
	}
	if primary == 0 {
		return fmt.Errorf("group %q needs at least one server that is not a backup", g.Name)
	}
	if g.MaxFails < 0 || g.MaxFails > 100 {
		return fmt.Errorf("max fails must be between 0 and 100")
	}
	if g.FailTimeout != "" && !reDuration.MatchString(g.FailTimeout) {
		return fmt.Errorf("fail timeout %q must be a duration such as 10s", g.FailTimeout)
	}
	return nil
}

func validMethod(m string) bool {
	for _, x := range Methods {
		if m == x {
			return true
		}
	}
	return false
}

// ValidateAddress accepts host:port with a plain host name or IP address.
func ValidateAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%q is not a host:port address", addr)
	}
	if host == "" || strings.ContainsAny(host, " \t;{}\"'$#/\\") {
		return fmt.Errorf("%q has an invalid host", addr)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("%q has an invalid port", addr)
	}
	return nil
}

// ParseServers parses "10.0.0.1:80, 10.0.0.2:80 weight=2, 10.0.0.3:80 backup".
func ParseServers(input string) ([]Server, error) {
	var servers []Server
	for _, part := range strings.Split(input, ",") {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		s := Server{Address: fields[0]}
		if err := ValidateAddress(s.Address); err != nil {
			return nil, err
		}
		for _, f := range fields[1:] {
			switch {
			case f == "backup":
				s.Backup = true
			case strings.HasPrefix(f, "weight="):
				w, err := strconv.Atoi(strings.TrimPrefix(f, "weight="))
				if err != nil || w < 1 || w > 1000 {
					return nil, fmt.Errorf("%q: weight must be between 1 and 1000", part)
				}
				s.Weight = w
			default:
				return nil, fmt.Errorf("%q: unknown option %q (use weight=N or backup)", strings.TrimSpace(part), f)
			}
		}
		servers = append(servers, s)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("enter at least one server, e.g. 10.0.0.11:3000")
	}
	return servers, nil
}

// FormatServers is the inverse of ParseServers.
func FormatServers(servers []Server) string {
	parts := make([]string, len(servers))
	for i, s := range servers {
		parts[i] = s.Address
		if s.Weight > 1 {
			parts[i] += " weight=" + strconv.Itoa(s.Weight)
		}
		if s.Backup {
			parts[i] += " backup"
		}
	}
	return strings.Join(parts, ", ")
}

// Render produces the http-level file content.
func Render(g Group) (string, error) {
	if err := g.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Upstream group: %s\n", nginx.ManagedMarker, g.Name)
	fmt.Fprintf(&b, "upstream %s {\n", nginx.UpstreamGroupName(g.Name))
	if g.Method != RoundRobin {
		fmt.Fprintf(&b, "    %s;\n", g.Method)
	}
	for _, s := range g.Servers {
		fmt.Fprintf(&b, "    server %s", s.Address)
		if s.Weight > 1 {
			fmt.Fprintf(&b, " weight=%d", s.Weight)
		}
		fmt.Fprintf(&b, " max_fails=%d", g.MaxFails)
		if g.FailTimeout != "" {
			fmt.Fprintf(&b, " fail_timeout=%s", g.FailTimeout)
		}
		if s.Backup {
			b.WriteString(" backup")
		}
		b.WriteString(";\n")
	}
	b.WriteString("}\n")
	return b.String(), nil
}

var (
	reServer      = regexp.MustCompile(`(?m)^\s*server\s+(\S+)([^;]*);`)
	reMethod      = regexp.MustCompile(`(?m)^\s*(least_conn|ip_hash)\s*;`)
	reWeight      = regexp.MustCompile(`weight=(\d+)`)
	reMaxFails    = regexp.MustCompile(`max_fails=(\d+)`)
	reFailTimeout = regexp.MustCompile(`fail_timeout=(\S+)`)
)

// Parse reconstructs a Group from content produced by Render.
func Parse(name, content string) Group {
	g := Group{Name: name, Method: RoundRobin}
	if m := reMethod.FindStringSubmatch(content); m != nil {
		g.Method = m[1]
	}
	for _, m := range reServer.FindAllStringSubmatch(content, -1) {
		s := Server{Address: m[1], Backup: strings.Contains(m[2], "backup")}
		if w := reWeight.FindStringSubmatch(m[2]); w != nil {
			s.Weight, _ = strconv.Atoi(w[1])
		}
		if f := reMaxFails.FindStringSubmatch(m[2]); f != nil {
			g.MaxFails, _ = strconv.Atoi(f[1])
		}
		if f := reFailTimeout.FindStringSubmatch(m[2]); f != nil {
			g.FailTimeout = f[1]
		}
		g.Servers = append(g.Servers, s)
	}
	return g
}

// Service manages upstream groups on disk.
type Service struct {
	Dir     string // http-level snippets directory
	Manager *nginx.Manager
	Writer  *snippet.Writer
}

// Path returns the file of the group called name.
func (s *Service) Path(name string) string {
	return filepath.Join(s.Dir, nginx.UpstreamFileName(name))
}

// Exists reports whether a group called name has been created.
func (s *Service) Exists(name string) bool {
	_, err := os.Stat(s.Path(name))
	return err == nil
}

// Get loads a single group.
func (s *Service) Get(name string) (Group, error) {
	data, err := os.ReadFile(s.Path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return Group{}, fmt.Errorf("upstream group %q does not exist", name)
		}
		return Group{}, fmt.Errorf("reading upstream group %q: %w", name, err)
	}
	return Parse(name, string(data)), nil
}

// List returns every GoNix upstream group, sorted by name.
func (s *Service) List() ([]Group, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", s.Dir, err)
	}
	var groups []Group
	for _, e := range entries {
		file := e.Name()
		if e.IsDir() || !strings.HasPrefix(file, nginx.UpstreamFilePrefix) || !strings.HasSuffix(file, ".conf") {
			continue
		}
		if g, err := s.Get(strings.TrimSuffix(strings.TrimPrefix(file, nginx.UpstreamFilePrefix), ".conf")); err == nil {
			groups = append(groups, g)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups, nil
}

// Create writes a new group.
func (s *Service) Create(ctx context.Context, g Group) error {
	if s.Exists(g.Name) {
		return fmt.Errorf("an upstream group named %q already exists", g.Name)
	}
	return s.write(ctx, g, "upstream_group_created")
}

// Update rewrites an existing group.
func (s *Service) Update(ctx context.Context, g Group) error {
	if !s.Exists(g.Name) {
		return fmt.Errorf("upstream group %q does not exist", g.Name)
	}
	return s.write(ctx, g, "upstream_group_updated")
}

func (s *Service) write(ctx context.Context, g Group, action string) error {
	content, err := Render(g)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.Dir, err)
	}
	return s.Writer.Write(ctx, action, g.Name, s.Path(g.Name), content)
}

// Delete removes a group. It refuses while any managed host still uses it.
func (s *Service) Delete(ctx context.Context, name string) error {
	users, err := s.UsedBy(name)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return fmt.Errorf("upstream group %q is still used by: %s", name, strings.Join(users, ", "))
	}
	return s.Writer.Remove(ctx, "upstream_group_deleted", name, s.Path(name))
}

// UsedBy returns "host location" for every managed location proxying to
// the group called name.
func (s *Service) UsedBy(name string) ([]string, error) {
	if s.Manager == nil {
		return nil, nil
	}
	hostList, err := s.Manager.ListHosts()
	if err != nil {
		return nil, err
	}
	var users []string
	for _, h := range hostList {
		for _, loc := range h.Locations {
			if loc.Proxy != nil && loc.Proxy.UpstreamGroup == name {
				users = append(users, h.ServerName+" "+loc.Path)
			}
		}
	}
	return users, nil
}
