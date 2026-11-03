// Package geoblock restricts hosts by the client's country, using the
// third-party geoip2 Nginx module and a MaxMind country database.
//
// Three kinds of files are involved:
//
//   - gonix-geoip.conf (http level): reads the database once into
//     $gonix_country_code. Written by Setup.
//   - gonix-geo-<list>.conf (http level): maps the country code to 1 when
//     the client must be refused.
//   - gonix-geo-<list>.conf (snippets): `if (...) { return 403; }`, which
//     hosts include at server level.
//
// Clients whose country is unknown (private addresses, missing database
// entries) are allowed by blocklists and refused by allowlists.
package geoblock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

// SetupFileName is the http-level file loading the GeoIP database.
const SetupFileName = "gonix-geoip.conf"

// CountryVar holds the client's ISO 3166-1 alpha-2 country code.
const CountryVar = "gonix_country_code"

// Mode decides what happens to countries that are not listed.
type Mode string

const (
	Allowlist Mode = "allowlist" // only listed countries may connect
	Blocklist Mode = "blocklist" // listed countries are refused
)

// List is a named list of country codes.
type List struct {
	Name      string
	Mode      Mode
	Countries []string // upper-case ISO 3166-1 alpha-2 codes
}

var reCountry = regexp.MustCompile(`^[A-Z]{2}$`)

// Validate checks that the list renders to safe Nginx syntax.
func (l List) Validate() error {
	if !nginx.ValidCountryListName(l.Name) {
		return fmt.Errorf("list name %q may only contain letters, digits and '_'", l.Name)
	}
	if l.Mode != Allowlist && l.Mode != Blocklist {
		return fmt.Errorf("list %q has unknown mode %q", l.Name, l.Mode)
	}
	if len(l.Countries) == 0 {
		return fmt.Errorf("list %q must contain at least one country", l.Name)
	}
	for _, c := range l.Countries {
		if !reCountry.MatchString(c) {
			return fmt.Errorf("%q is not a two-letter country code (e.g. DE, US)", c)
		}
	}
	return nil
}

// ParseCountries parses "de, us fr" into sorted, de-duplicated upper-case
// codes.
func ParseCountries(input string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, f := range strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		c := strings.ToUpper(f)
		if !reCountry.MatchString(c) {
			return nil, fmt.Errorf("%q is not a two-letter country code (e.g. DE, US)", f)
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("enter at least one country code, e.g. DE")
	}
	sort.Strings(out)
	return out, nil
}

func varName(name string) string { return "gonix_geo_" + name }

// RenderMap produces the http-level map of a list.
func RenderMap(l List) (string, error) {
	if err := l.Validate(); err != nil {
		return "", err
	}
	listed, rest := "0", "1"
	if l.Mode == Blocklist {
		listed, rest = "1", "0"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Country list: %s (%s)\n", nginx.ManagedMarker, l.Name, l.Mode)
	b.WriteString("# 1 = refuse the client.\n")
	fmt.Fprintf(&b, "map $%s $%s {\n    default %s;\n", CountryVar, varName(l.Name), rest)
	for _, c := range l.Countries {
		fmt.Fprintf(&b, "    %s %s;\n", c, listed)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// RenderCheck produces the server-level snippet hosts include.
func RenderCheck(l List) (string, error) {
	if err := l.Validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s Country list: %s\nif ($%s) {\n    return 403;\n}\n", nginx.ManagedMarker, l.Name, varName(l.Name)), nil
}

// RenderSetup produces the http-level file loading database.
func RenderSetup(database string) (string, error) {
	if !filepath.IsAbs(database) || strings.ContainsAny(database, " \t\n;{}\"'$#") {
		return "", fmt.Errorf("GeoIP database %q must be an absolute path without spaces or any of ; { } \" ' $ #", database)
	}
	return fmt.Sprintf("%s GeoIP country lookup for country lists.\ngeoip2 %s {\n    $%s country iso_code;\n}\n",
		nginx.ManagedMarker, database, CountryVar), nil
}

var (
	reMode      = regexp.MustCompile(`Country list: \S+ \((\w+)\)`)
	reMapEntry  = regexp.MustCompile(`(?m)^\s+([A-Z]{2})\s+[01];`)
	reGeoipPath = regexp.MustCompile(`geoip2\s+(\S+)\s*\{`)
)

// Parse reconstructs a List from its http-level map.
func Parse(name, mapContent string) List {
	l := List{Name: name, Mode: Blocklist}
	if m := reMode.FindStringSubmatch(mapContent); m != nil {
		l.Mode = Mode(m[1])
	}
	for _, m := range reMapEntry.FindAllStringSubmatch(mapContent, -1) {
		l.Countries = append(l.Countries, m[1])
	}
	return l
}

// Service manages the GeoIP setup and country lists.
type Service struct {
	HTTPDir     string
	SnippetsDir string
	Database    string
	Manager     *nginx.Manager
	Writer      *snippet.Writer
}

func (s *Service) setupPath() string { return filepath.Join(s.HTTPDir, SetupFileName) }

// MapPath and CheckPath return the two files of the list called name.
func (s *Service) MapPath(name string) string {
	return filepath.Join(s.HTTPDir, nginx.CountryListFileName(name))
}

func (s *Service) CheckPath(name string) string {
	return filepath.Join(s.SnippetsDir, nginx.CountryListFileName(name))
}

// Status reports whether GeoIP lookups are set up and which database they
// read.
func (s *Service) Status() (ready bool, database string) {
	data, err := os.ReadFile(s.setupPath())
	if err != nil {
		return false, ""
	}
	if m := reGeoipPath.FindStringSubmatch(string(data)); m != nil {
		return true, m[1]
	}
	return true, ""
}

// SetUp writes the http-level file that loads the database. `nginx -t`
// rejects it, and it is removed again, when the geoip2 module is not
// loaded or the database cannot be opened.
func (s *Service) SetUp(ctx context.Context) error {
	if _, err := os.Stat(s.Database); err != nil {
		return fmt.Errorf("GeoIP database %s not found; install one (e.g. with geoipupdate) or set geoip.database in /etc/gonix/gonix.yaml", s.Database)
	}
	content, err := RenderSetup(s.Database)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.HTTPDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.HTTPDir, err)
	}
	if err := s.Writer.Write(ctx, "geoip_setup", "geoip", s.setupPath(), content); err != nil {
		return fmt.Errorf("%w\n\nIf Nginx reports an unknown \"geoip2\" directive, install and load the geoip2 module (Debian/Ubuntu: libnginx-mod-http-geoip2)", err)
	}
	return nil
}

// Exists reports whether a list called name has been created.
func (s *Service) Exists(name string) bool {
	_, err := os.Stat(s.CheckPath(name))
	return err == nil
}

// Get loads a single list.
func (s *Service) Get(name string) (List, error) {
	data, err := os.ReadFile(s.MapPath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return List{}, fmt.Errorf("country list %q does not exist", name)
		}
		return List{}, err
	}
	return Parse(name, string(data)), nil
}

// List returns every country list, sorted by name.
func (s *Service) List() ([]List, error) {
	entries, err := os.ReadDir(s.SnippetsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []List
	for _, e := range entries {
		name, ok := strings.CutPrefix(e.Name(), nginx.CountryListFilePrefix)
		if !ok {
			continue
		}
		if name, ok = strings.CutSuffix(name, ".conf"); !ok || !nginx.ValidCountryListName(name) {
			continue
		}
		if l, err := s.Get(name); err == nil {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save creates or updates a list. Lists can only be created once GeoIP is
// set up, since their map reads $gonix_country_code.
func (s *Service) Save(ctx context.Context, l List, create bool) error {
	if create {
		if ready, _ := s.Status(); !ready {
			return fmt.Errorf("set up GeoIP lookups first")
		}
		if s.Exists(l.Name) {
			return fmt.Errorf("a country list named %q already exists", l.Name)
		}
	} else if !s.Exists(l.Name) {
		return fmt.Errorf("country list %q does not exist", l.Name)
	}
	m, err := RenderMap(l)
	if err != nil {
		return err
	}
	c, err := RenderCheck(l)
	if err != nil {
		return err
	}
	action := "country_list_updated"
	if create {
		action = "country_list_created"
	}
	return s.Writer.Apply(ctx, action, l.Name,
		snippet.File{Path: s.MapPath(l.Name), Content: &m},
		snippet.File{Path: s.CheckPath(l.Name), Content: &c},
	)
}

// Delete removes a list. It refuses while any managed host still uses it.
func (s *Service) Delete(ctx context.Context, name string) error {
	users, err := s.UsedBy(name)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return fmt.Errorf("country list %q is still used by: %s", name, strings.Join(users, ", "))
	}
	return s.Writer.Apply(ctx, "country_list_deleted", name,
		snippet.File{Path: s.CheckPath(name)},
		snippet.File{Path: s.MapPath(name)},
	)
}

// UsedBy returns the server names of managed hosts using the list.
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
		if h.CountryList == name {
			users = append(users, h.ServerName)
		}
	}
	return users, nil
}
