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
}

// NewRenderer builds a Renderer. accessLogDir/errorLogDir are typically both
// /var/log/nginx; accessListFileDir is where htpasswd files for access lists
// are stored.
func NewRenderer(accessLogDir, errorLogDir, accessListFileDir string) (*Renderer, error) {
	tmpl, err := template.New("host.tmpl").ParseFS(templates.FS, "host.tmpl", "location.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	return &Renderer{
		tmpl:              tmpl,
		accessLogDir:      accessLogDir,
		errorLogDir:       errorLogDir,
		accessListFileDir: accessListFileDir,
	}, nil
}

// renderView is the data handed to host.tmpl.
type renderView struct {
	Host           Host
	AccessLogPath  string
	ErrorLogPath   string
	AccessListFile string
}

func (r *Renderer) htpasswdPath(name string) string {
	if name == "" {
		return ""
	}
	return filepath.Join(r.accessListFileDir, name+".htpasswd")
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
		rendered.Locations[i] = l
	}

	view := renderView{
		Host:           rendered,
		AccessLogPath:  filepath.Join(r.accessLogDir, h.ServerName+".access.log"),
		ErrorLogPath:   filepath.Join(r.errorLogDir, h.ServerName+".error.log"),
		AccessListFile: r.htpasswdPath(h.AccessListName),
	}

	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, "host.tmpl", view); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}
