// Package iplist implements reusable, named IP access lists: snippets of
// Nginx allow/deny rules that hosts or individual locations include. A list
// is either an allowlist (listed addresses only, everyone else denied) or a
// blocklist (listed addresses denied, everyone else allowed).
package iplist

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

// Mode decides what happens to addresses that are not listed.
type Mode string

const (
	// Allowlist allows only the listed addresses.
	Allowlist Mode = "allowlist"
	// Blocklist denies the listed addresses and allows everyone else.
	Blocklist Mode = "blocklist"
)

// List is a named IP access list.
type List struct {
	Name    string
	Mode    Mode
	Entries []string // IP addresses or CIDR ranges
}

// Validate checks that the list can be rendered to safe Nginx syntax.
func (l List) Validate() error {
	if !nginx.ValidSnippetName(l.Name) {
		return fmt.Errorf("list name %q may only contain letters, digits, '-' and '_'", l.Name)
	}
	if l.Mode != Allowlist && l.Mode != Blocklist {
		return fmt.Errorf("list %q has unknown mode %q", l.Name, l.Mode)
	}
	if len(l.Entries) == 0 {
		return fmt.Errorf("list %q must contain at least one IP address or range", l.Name)
	}
	for _, e := range l.Entries {
		if err := ValidateEntry(e); err != nil {
			return err
		}
	}
	return nil
}

// ValidateEntry accepts a single IPv4/IPv6 address or CIDR range.
func ValidateEntry(e string) error {
	if strings.Contains(e, "/") {
		if _, _, err := net.ParseCIDR(e); err != nil {
			return fmt.Errorf("%q is not a valid CIDR range (e.g. 192.168.1.0/24)", e)
		}
		return nil
	}
	if net.ParseIP(e) == nil {
		return fmt.Errorf("%q is not a valid IP address", e)
	}
	return nil
}

// ParseEntries parses a user supplied list such as "10.0.0.1, 192.168.0.0/16"
// into validated, de-duplicated entries, keeping their order.
func ParseEntries(input string) ([]string, error) {
	fields := strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	seen := map[string]bool{}
	var entries []string
	for _, f := range fields {
		if err := ValidateEntry(f); err != nil {
			return nil, err
		}
		if !seen[f] {
			seen[f] = true
			entries = append(entries, f)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("enter at least one IP address or range, e.g. 192.168.1.0/24")
	}
	return entries, nil
}

// Contains reports whether ip is covered by one of the list's entries.
func (l List) Contains(ip string) bool {
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	for _, e := range l.Entries {
		if _, n, err := net.ParseCIDR(e); err == nil {
			if n.Contains(addr) {
				return true
			}
		} else if other := net.ParseIP(e); other != nil && other.Equal(addr) {
			return true
		}
	}
	return false
}

// Render produces the snippet file content.
func Render(l List) (string, error) {
	if err := l.Validate(); err != nil {
		return "", err
	}
	listed, rest := "allow", "deny"
	if l.Mode == Blocklist {
		listed, rest = "deny", "allow"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s IP access list: %s (%s)\n", nginx.ManagedMarker, l.Name, l.Mode)
	b.WriteString("# Edit it from GoNix: rules are evaluated top to bottom, first match wins.\n")
	for _, e := range l.Entries {
		fmt.Fprintf(&b, "%s %s;\n", listed, e)
	}
	fmt.Fprintf(&b, "%s all;\n", rest)
	return b.String(), nil
}

var reRule = regexp.MustCompile(`(?m)^\s*(allow|deny)\s+([^;\s]+)\s*;`)

// Parse reconstructs a List from content produced by Render.
func Parse(name, content string) List {
	l := List{Name: name, Mode: Allowlist}
	for _, m := range reRule.FindAllStringSubmatch(content, -1) {
		if m[2] == "all" {
			if m[1] == "allow" {
				l.Mode = Blocklist
			}
			continue
		}
		l.Entries = append(l.Entries, m[2])
	}
	return l
}

// Store manages list files under a single directory, each named
// nginx.IPListFileName(name).
type Store struct {
	Directory string
}

// NewStore returns a Store rooted at dir, creating it if necessary.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating snippets directory: %w", err)
	}
	return &Store{Directory: dir}, nil
}

// Path returns the snippet file path for name.
func (s *Store) Path(name string) string {
	return filepath.Join(s.Directory, nginx.IPListFileName(name))
}

// Exists reports whether a list called name has been created.
func (s *Store) Exists(name string) bool {
	_, err := os.Stat(s.Path(name))
	return err == nil
}

// Get loads and parses a single list.
func (s *Store) Get(name string) (List, error) {
	data, err := os.ReadFile(s.Path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return List{}, fmt.Errorf("IP access list %q does not exist", name)
		}
		return List{}, fmt.Errorf("reading IP access list %q: %w", name, err)
	}
	return Parse(name, string(data)), nil
}

// List returns every GoNix-generated IP list, sorted by name.
func (s *Store) List() ([]List, error) {
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading snippets directory: %w", err)
	}
	var lists []List
	for _, e := range entries {
		file := e.Name()
		if e.IsDir() || !strings.HasPrefix(file, nginx.IPListFilePrefix) || !strings.HasSuffix(file, ".conf") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(file, nginx.IPListFilePrefix), ".conf")
		if !nginx.ValidSnippetName(name) {
			continue
		}
		if l, err := s.Get(name); err == nil {
			lists = append(lists, l)
		}
	}
	sort.Slice(lists, func(i, j int) bool { return lists[i].Name < lists[j].Name })
	return lists, nil
}

// Service applies list changes safely: a list may be included by any number
// of hosts, so every write goes through snippet.Writer (nginx -t, restore
// on failure, reload, audit).
type Service struct {
	Store   *Store
	Manager *nginx.Manager // used to find hosts that include a list
	Writer  *snippet.Writer
}

// Create writes a brand new list.
func (s *Service) Create(ctx context.Context, l List) error {
	if s.Store.Exists(l.Name) {
		return fmt.Errorf("an IP access list named %q already exists", l.Name)
	}
	return s.write(ctx, l, "ip_list_created")
}

// Update rewrites an existing list; every host including it picks up the
// change.
func (s *Service) Update(ctx context.Context, l List) error {
	if !s.Store.Exists(l.Name) {
		return fmt.Errorf("IP access list %q does not exist", l.Name)
	}
	return s.write(ctx, l, "ip_list_updated")
}

// AddEntry appends ip to the list called name, if it is not already listed
// verbatim. Used to block an address straight from the log viewer.
func (s *Service) AddEntry(ctx context.Context, name, ip string) error {
	if err := ValidateEntry(ip); err != nil {
		return err
	}
	l, err := s.Store.Get(name)
	if err != nil {
		return err
	}
	for _, e := range l.Entries {
		if e == ip {
			return nil
		}
	}
	l.Entries = append(l.Entries, ip)
	return s.write(ctx, l, "ip_list_updated")
}

func (s *Service) write(ctx context.Context, l List, action string) error {
	content, err := Render(l)
	if err != nil {
		return err
	}
	return s.Writer.Write(ctx, action, l.Name, s.Store.Path(l.Name), content)
}

// Delete removes a list. It refuses while any managed host still uses it.
func (s *Service) Delete(ctx context.Context, name string) error {
	users, err := s.UsedBy(name)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return fmt.Errorf("IP access list %q is still used by: %s", name, strings.Join(users, ", "))
	}
	return s.Writer.Remove(ctx, "ip_list_deleted", name, s.Store.Path(name))
}

// UsedBy returns "host" or "host location" for every managed host or
// location that includes the list called name.
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
		if h.IPListName == name {
			users = append(users, h.ServerName)
		}
		for _, loc := range h.Locations {
			if loc.IPListName == name {
				users = append(users, h.ServerName+" "+loc.Path)
			}
		}
	}
	return users, nil
}
