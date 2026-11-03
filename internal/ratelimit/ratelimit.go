// Package ratelimit implements named rate limits. Nginx splits a rate limit
// across two contexts: the shared-memory zone is declared once at http level
// (limit_req_zone), and hosts or locations then apply it (limit_req). GoNix
// keeps both halves under one name and always writes or removes them
// together, so they can never get out of sync.
package ratelimit

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
)

// Limit is a named rate limit, applied per client IP address.
type Limit struct {
	Name    string
	Rate    int    // requests per Per
	Per     string // "s" (second) or "m" (minute)
	Burst   int    // requests allowed above Rate before rejecting; 0 = none
	NoDelay bool   // serve burst requests immediately instead of queueing them
	Conn    int    // maximum concurrent connections per client; 0 = unlimited
}

// Presets are offered when creating a limit.
var Presets = []Limit{
	{Name: "general", Rate: 30, Per: "s", Burst: 60, NoDelay: true, Conn: 50},
	{Name: "api", Rate: 10, Per: "s", Burst: 20, NoDelay: true},
	{Name: "login", Rate: 5, Per: "m", Burst: 5},
}

// Validate checks that the limit renders to safe Nginx syntax.
func (l Limit) Validate() error {
	if !nginx.ValidSnippetName(l.Name) {
		return fmt.Errorf("rate limit name %q may only contain letters, digits, '-' and '_'", l.Name)
	}
	if l.Rate < 1 || l.Rate > 100000 {
		return fmt.Errorf("rate must be between 1 and 100000")
	}
	if l.Per != "s" && l.Per != "m" {
		return fmt.Errorf("rate unit must be per second (s) or per minute (m)")
	}
	if l.Burst < 0 || l.Burst > 100000 {
		return fmt.Errorf("burst must be between 0 and 100000")
	}
	if l.Conn < 0 || l.Conn > 100000 {
		return fmt.Errorf("connection limit must be between 0 and 100000")
	}
	return nil
}

// Describe summarizes the limit, e.g. "10 req/s, burst 20, 50 connections".
func (l Limit) Describe() string {
	unit := "s"
	if l.Per == "m" {
		unit = "min"
	}
	d := fmt.Sprintf("%d req/%s", l.Rate, unit)
	if l.Burst > 0 {
		d += fmt.Sprintf(", burst %d", l.Burst)
		if l.NoDelay {
			d += " (no delay)"
		}
	}
	if l.Conn > 0 {
		d += fmt.Sprintf(", %d connections", l.Conn)
	}
	return d
}

func (l Limit) reqZone() string  { return "gonix_" + l.Name }
func (l Limit) connZone() string { return "gonix_" + l.Name + "_conn" }

// RenderZone produces the http-level file declaring the shared zones.
func RenderZone(l Limit) (string, error) {
	if err := l.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Rate limit zone: %s\n", nginx.ManagedMarker, l.Name)
	fmt.Fprintf(&b, "limit_req_zone $binary_remote_addr zone=%s:10m rate=%dr/%s;\n", l.reqZone(), l.Rate, l.Per)
	if l.Conn > 0 {
		fmt.Fprintf(&b, "limit_conn_zone $binary_remote_addr zone=%s:10m;\n", l.connZone())
	}
	return b.String(), nil
}

// RenderApply produces the snippet hosts and locations include.
func RenderApply(l Limit) (string, error) {
	if err := l.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Rate limit: %s (%s)\n", nginx.ManagedMarker, l.Name, l.Describe())
	b.WriteString("# Clients over the limit get 429 Too Many Requests.\n")
	b.WriteString("limit_req zone=" + l.reqZone())
	if l.Burst > 0 {
		fmt.Fprintf(&b, " burst=%d", l.Burst)
		if l.NoDelay {
			b.WriteString(" nodelay")
		}
	}
	b.WriteString(";\nlimit_req_status 429;\n")
	if l.Conn > 0 {
		fmt.Fprintf(&b, "limit_conn %s %d;\nlimit_conn_status 429;\n", l.connZone(), l.Conn)
	}
	return b.String(), nil
}

var (
	reReqZone  = regexp.MustCompile(`limit_req_zone\s+\S+\s+zone=\S+?:\S+\s+rate=(\d+)r/([sm])\s*;`)
	reConnZone = regexp.MustCompile(`limit_conn_zone\s`)
	reReq      = regexp.MustCompile(`limit_req\s+zone=\S+?(?:\s+burst=(\d+))?(\s+nodelay)?\s*;`)
	reConn     = regexp.MustCompile(`limit_conn\s+\S+\s+(\d+)\s*;`)
)

// Parse reconstructs a Limit from the zone file and the apply snippet.
func Parse(name, zone, apply string) Limit {
	l := Limit{Name: name, Per: "s"}
	if m := reReqZone.FindStringSubmatch(zone); m != nil {
		l.Rate, _ = strconv.Atoi(m[1])
		l.Per = m[2]
	}
	if m := reReq.FindStringSubmatch(apply); m != nil {
		l.Burst, _ = strconv.Atoi(m[1])
		l.NoDelay = m[2] != ""
	}
	if reConnZone.MatchString(zone) {
		if m := reConn.FindStringSubmatch(apply); m != nil {
			l.Conn, _ = strconv.Atoi(m[1])
		}
	}
	return l
}

// Service manages rate limits on disk.
type Service struct {
	SnippetsDir string // apply snippets, included by hosts
	HTTPDir     string // zone files, e.g. /etc/nginx/conf.d
	Manager     *nginx.Manager
	Writer      *snippet.Writer
}

// ZonePath and ApplyPath return the two files of the limit called name.
func (s *Service) ZonePath(name string) string {
	return filepath.Join(s.HTTPDir, nginx.RateLimitFileName(name))
}

func (s *Service) ApplyPath(name string) string {
	return filepath.Join(s.SnippetsDir, nginx.RateLimitFileName(name))
}

// Exists reports whether a limit called name has been created.
func (s *Service) Exists(name string) bool {
	_, err := os.Stat(s.ApplyPath(name))
	return err == nil
}

// Get loads a single limit.
func (s *Service) Get(name string) (Limit, error) {
	apply, err := os.ReadFile(s.ApplyPath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return Limit{}, fmt.Errorf("rate limit %q does not exist", name)
		}
		return Limit{}, fmt.Errorf("reading rate limit %q: %w", name, err)
	}
	zone, err := os.ReadFile(s.ZonePath(name))
	if err != nil {
		return Limit{}, fmt.Errorf("reading rate limit zone %q: %w", name, err)
	}
	return Parse(name, string(zone), string(apply)), nil
}

// List returns every GoNix rate limit, sorted by name.
func (s *Service) List() ([]Limit, error) {
	entries, err := os.ReadDir(s.SnippetsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading snippets directory: %w", err)
	}
	var limits []Limit
	for _, e := range entries {
		file := e.Name()
		if e.IsDir() || !strings.HasPrefix(file, nginx.RateLimitFilePrefix) || !strings.HasSuffix(file, ".conf") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(file, nginx.RateLimitFilePrefix), ".conf")
		if l, err := s.Get(name); err == nil {
			limits = append(limits, l)
		}
	}
	sort.Slice(limits, func(i, j int) bool { return limits[i].Name < limits[j].Name })
	return limits, nil
}

// Create writes a new limit.
func (s *Service) Create(ctx context.Context, l Limit) error {
	if s.Exists(l.Name) {
		return fmt.Errorf("a rate limit named %q already exists", l.Name)
	}
	return s.write(ctx, l, "rate_limit_created")
}

// Update rewrites an existing limit; every host using it picks up the
// change.
func (s *Service) Update(ctx context.Context, l Limit) error {
	if !s.Exists(l.Name) {
		return fmt.Errorf("rate limit %q does not exist", l.Name)
	}
	return s.write(ctx, l, "rate_limit_updated")
}

func (s *Service) write(ctx context.Context, l Limit, action string) error {
	zone, err := RenderZone(l)
	if err != nil {
		return err
	}
	apply, err := RenderApply(l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.HTTPDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.HTTPDir, err)
	}
	return s.Writer.Apply(ctx, action, l.Name,
		snippet.File{Path: s.ZonePath(l.Name), Content: &zone},
		snippet.File{Path: s.ApplyPath(l.Name), Content: &apply},
	)
}

// Delete removes a limit. It refuses while any managed host still uses it.
func (s *Service) Delete(ctx context.Context, name string) error {
	users, err := s.UsedBy(name)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return fmt.Errorf("rate limit %q is still used by: %s", name, strings.Join(users, ", "))
	}
	return s.Writer.Apply(ctx, "rate_limit_deleted", name,
		snippet.File{Path: s.ApplyPath(name)},
		snippet.File{Path: s.ZonePath(name)},
	)
}

// UsedBy returns "host" or "host location" for every managed host or
// location using the limit called name.
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
		if h.RateLimit == name {
			users = append(users, h.ServerName)
		}
		for _, loc := range h.Locations {
			if loc.RateLimit == name {
				users = append(users, h.ServerName+" "+loc.Path)
			}
		}
	}
	return users, nil
}
