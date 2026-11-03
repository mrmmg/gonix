package nginx

import (
	"bytes"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/mrmmg/gonix/templates"
)

// Renderer turns a Host domain object into a plain-text Nginx configuration
// file. It knows nothing about the filesystem beyond the paths it is given.
type Renderer struct {
	tmpl              *template.Template
	accessLogDir      string
	errorLogDir       string
	accessListFileDir string
	snippetsDir       string

	// Build selects version-dependent syntax; the zero value renders
	// syntax every supported Nginx version accepts.
	Build BuildInfo
}

// NewRenderer builds a Renderer. accessLogDir/errorLogDir are typically both
// /var/log/nginx; accessListFileDir is where htpasswd files for access lists
// are stored; snippetsDir is where error pages snippets are stored.
func NewRenderer(accessLogDir, errorLogDir, accessListFileDir, snippetsDir string) (*Renderer, error) {
	tmpl, err := template.New("host.tmpl").ParseFS(templates.FS, "host.tmpl", "location.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	return &Renderer{
		tmpl:              tmpl,
		accessLogDir:      accessLogDir,
		errorLogDir:       errorLogDir,
		accessListFileDir: accessListFileDir,
		snippetsDir:       snippetsDir,
	}, nil
}

// renderView is the data handed to host.tmpl.
type renderView struct {
	Host           Host
	AccessLogPath  string
	ErrorLogPath   string
	AccessListFile string
	IPListFile     string
	RateLimitFile  string
	ErrorPagesFile string
	PresetFiles    []string
	HTTP2Directive bool

	MaintenanceVar     string
	MaintenanceFile    string
	ErrorPageURIPrefix string
}

func (r *Renderer) htpasswdPath(name string) string {
	if name == "" {
		return ""
	}
	return filepath.Join(r.accessListFileDir, name+".htpasswd")
}

func (r *Renderer) ipListPath(name string) string {
	return r.snippetPath(IPListFileName, name)
}

// snippetPath resolves a snippet name to its file in the snippets
// directory, or "" when name is empty.
func (r *Renderer) snippetPath(fileName func(string) string, name string) string {
	if name == "" {
		return ""
	}
	return filepath.Join(r.snippetsDir, fileName(name))
}

// Render produces the full Nginx server-block configuration for host h.
func (r *Renderer) Render(h Host) (string, error) {
	if err := h.Validate(); err != nil {
		return "", fmt.Errorf("invalid host: %w", err)
	}

	// Resolve access-list names to htpasswd file paths for rendering,
	// without mutating the caller's host.
	rendered := h
	rendered.Locations = make([]Location, len(h.Locations))
	for i, l := range h.Locations {
		l.AccessListName = r.htpasswdPath(l.AccessListName)
		l.IPListName = r.ipListPath(l.IPListName)
		l.RateLimit = r.snippetPath(RateLimitFileName, l.RateLimit)
		rendered.Locations[i] = l
	}

	view := renderView{
		Host:           rendered,
		AccessLogPath:  filepath.Join(r.accessLogDir, h.ServerName+".access.log"),
		ErrorLogPath:   filepath.Join(r.errorLogDir, h.ServerName+".error.log"),
		AccessListFile: r.htpasswdPath(h.AccessListName),
		IPListFile:     r.ipListPath(h.IPListName),
		RateLimitFile:  r.snippetPath(RateLimitFileName, h.RateLimit),
		HTTP2Directive: r.Build.HTTP2Directive(),
	}
	if h.Maintenance && h.Mode != ModeRedirect {
		view.MaintenanceVar = MaintenanceVar(h.ServerName)
		view.MaintenanceFile = MaintenanceFileName(h.ServerName)
		view.ErrorPageURIPrefix = ErrorPageURIPrefix
	}
	for _, p := range h.Presets {
		view.PresetFiles = append(view.PresetFiles, filepath.Join(r.snippetsDir, PresetFileName(p)))
	}
	if h.ErrorPagesSnippet != "" {
		view.ErrorPagesFile = filepath.Join(r.snippetsDir, SnippetFileName(h.ErrorPagesSnippet))
	}

	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, "host.tmpl", view); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}
