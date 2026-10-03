// Package errorpages implements reusable, named error page snippets: small
// Nginx configuration files that map HTTP error codes to static HTML pages
// (<pages directory>/<code>.html) and are included by hosts at server level.
package errorpages

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/templates"
)

// URIPrefix is the internal URI the error pages are served under. It is
// deliberately unusual so it never collides with a host's own paths.
const URIPrefix = "/__gonix_error_pages/"

// Snippet is a named mapping of HTTP error codes to HTML pages stored in a
// single directory.
type Snippet struct {
	Name      string
	Directory string // holds <code>.html for every code in Codes
	Codes     []int
}

// Validate checks that the snippet can be rendered to safe Nginx syntax.
func (s Snippet) Validate() error {
	if !nginx.ValidSnippetName(s.Name) {
		return fmt.Errorf("snippet name %q may only contain letters, digits, '-' and '_'", s.Name)
	}
	if err := ValidateDirectory(s.Directory); err != nil {
		return err
	}
	if len(s.Codes) == 0 {
		return fmt.Errorf("snippet %q must handle at least one error code", s.Name)
	}
	for _, c := range s.Codes {
		if err := validateCode(c); err != nil {
			return err
		}
	}
	return nil
}

// PagePath returns the HTML file that answers the given code.
func (s Snippet) PagePath(code int) string {
	return filepath.Join(s.Directory, strconv.Itoa(code)+".html")
}

// MissingPages lists the HTML files this snippet refers to that do not
// exist on disk. Nginx accepts the configuration regardless, but a missing
// page makes it fall back to its built-in error page.
func (s Snippet) MissingPages() []string {
	var missing []string
	for _, c := range s.Codes {
		p := s.PagePath(c)
		if _, err := os.Stat(p); err != nil {
			missing = append(missing, p)
		}
	}
	return missing
}

func validateCode(c int) error {
	if c < 400 || c > 599 {
		return fmt.Errorf("error code %d is out of range (400-599)", c)
	}
	return nil
}

// ValidateDirectory checks that dir is an absolute path that can be written
// into an Nginx alias directive verbatim.
func ValidateDirectory(dir string) error {
	if dir == "" {
		return fmt.Errorf("pages directory must not be empty")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("pages directory %q must be an absolute path", dir)
	}
	if strings.ContainsAny(dir, " \t\n;{}\"'$#") {
		return fmt.Errorf("pages directory %q must not contain spaces or any of ; { } \" ' $ #", dir)
	}
	return nil
}

// ParseCodes parses a user supplied list such as "502, 503 504" into a
// sorted, de-duplicated list of codes.
func ParseCodes(input string) ([]int, error) {
	fields := strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	seen := map[int]bool{}
	var codes []int
	for _, f := range fields {
		c, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("%q is not a valid HTTP status code", f)
		}
		if err := validateCode(c); err != nil {
			return nil, err
		}
		if !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	if len(codes) == 0 {
		return nil, fmt.Errorf("enter at least one error code, e.g. 502")
	}
	sort.Ints(codes)
	return codes, nil
}

// FormatCodes is the inverse of ParseCodes, e.g. "502, 503, 504".
func FormatCodes(codes []int) string {
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = strconv.Itoa(c)
	}
	return strings.Join(parts, ", ")
}

var (
	reErrorPage = regexp.MustCompile(`(?m)^\s*error_page\s+(\d+)\s`)
	reAlias     = regexp.MustCompile(`(?m)^\s*alias\s+([^;]+);`)
)

// Store manages snippet files under a single directory, each named
// nginx.SnippetFileName(name).
type Store struct {
	Directory string
	tmpl      *template.Template
}

// NewStore returns a Store rooted at dir, creating it if necessary.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating snippets directory: %w", err)
	}
	tmpl, err := template.ParseFS(templates.FS, "errorpages.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing error pages template: %w", err)
	}
	return &Store{Directory: dir, tmpl: tmpl}, nil
}

// Path returns the snippet file path for name.
func (s *Store) Path(name string) string {
	return filepath.Join(s.Directory, nginx.SnippetFileName(name))
}

// Exists reports whether a snippet called name has been created.
func (s *Store) Exists(name string) bool {
	_, err := os.Stat(s.Path(name))
	return err == nil
}

// List returns every GoNix-generated snippet, sorted by name.
func (s *Store) List() ([]Snippet, error) {
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading snippets directory: %w", err)
	}
	var snippets []Snippet
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, ok := snippetNameFromFile(e.Name())
		if !ok {
			continue
		}
		sn, err := s.Get(name)
		if err != nil {
			continue
		}
		snippets = append(snippets, sn)
	}
	sort.Slice(snippets, func(i, j int) bool { return snippets[i].Name < snippets[j].Name })
	return snippets, nil
}

func snippetNameFromFile(file string) (string, bool) {
	if !strings.HasPrefix(file, nginx.SnippetFilePrefix) || !strings.HasSuffix(file, ".conf") {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(file, nginx.SnippetFilePrefix), ".conf")
	return name, nginx.ValidSnippetName(name)
}

// Get loads and parses a single snippet.
func (s *Store) Get(name string) (Snippet, error) {
	data, err := os.ReadFile(s.Path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return Snippet{}, fmt.Errorf("error pages snippet %q does not exist", name)
		}
		return Snippet{}, fmt.Errorf("reading error pages snippet %q: %w", name, err)
	}
	return Parse(name, string(data)), nil
}

// Parse reconstructs a Snippet from content produced by Render.
func Parse(name, content string) Snippet {
	sn := Snippet{Name: name}
	if m := reAlias.FindStringSubmatch(content); m != nil {
		sn.Directory = strings.TrimSuffix(strings.TrimSpace(m[1]), "/")
	}
	for _, m := range reErrorPage.FindAllStringSubmatch(content, -1) {
		if c, err := strconv.Atoi(m[1]); err == nil {
			sn.Codes = append(sn.Codes, c)
		}
	}
	return sn
}

// Render produces the snippet file content.
func (s *Store) Render(sn Snippet) (string, error) {
	if err := sn.Validate(); err != nil {
		return "", err
	}
	view := struct {
		Snippet
		URIPrefix string
	}{sn, URIPrefix}
	view.Directory = strings.TrimSuffix(sn.Directory, "/")

	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, "errorpages.tmpl", view); err != nil {
		return "", fmt.Errorf("executing error pages template: %w", err)
	}
	return buf.String(), nil
}
