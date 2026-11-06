// Package lab builds the GoNix lab: a second Nginx, separate from the
// system's, with its own configuration directory, systemd unit and GoNix
// instance, filled with hand-written test hosts on demand. It is for trying
// GoNix (adopting, editing, refusals, rollbacks) without touching the real
// server, and is offered only when GONIX_LAB is set.
//
// Everything is generated from the templates in templates/lab.
package lab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/templates"
)

const (
	// EnvVar enables the Lab menu.
	EnvVar = "GONIX_LAB"
	// Instance is the GoNix instance the lab is managed as.
	Instance = "lab"
	// Unit is the lab's systemd unit.
	Unit = "gonix-lab"

	DefaultDir       = "/srv/gonix-lab"
	DefaultHTTPPort  = 8080
	DefaultHTTPSPort = 8443

	stateFile = "lab.json"
	// marker starts the instance file the lab writes, so it never takes
	// over an instance named "lab" that someone set up by hand.
	marker = "# GoNix lab instance."
)

// Enabled reports whether the Lab menu is offered.
func Enabled() bool {
	switch os.Getenv(EnvVar) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// State is what the lab records about itself in <Dir>/lab.json.
type State struct {
	Dir       string    `json:"dir"`
	HTTPPort  int       `json:"http_port"`
	HTTPSPort int       `json:"https_port"`
	BackendA  string    `json:"backend_a"`
	BackendB  string    `json:"backend_b"`
	Created   time.Time `json:"created"`
	Hosts     []Host    `json:"hosts"`
}

// Host is a test host the lab created.
type Host struct {
	Scenario string `json:"scenario"`
	Domain   string `json:"domain"`
	File     string `json:"file"`
}

// Options configure Setup.
type Options struct {
	Dir                 string
	HTTPPort, HTTPSPort int
	// Samples adds one test host of every scenario.
	Samples bool
}

// Lab creates, runs and removes the lab. The zero value is not usable; see
// New.
type Lab struct {
	InstanceFile string // the lab's GoNix instance configuration
	UnitDir      string // where the systemd unit goes
	Nginx        string // absolute path of the nginx binary
	// Systemctl runs systemctl; TestConfig runs nginx -t on a config file.
	Systemctl  func(ctx context.Context, args ...string) (string, error)
	TestConfig func(ctx context.Context, configFile string) (string, error)
	// PortFree reports whether a TCP port can be listened on.
	PortFree func(port int) bool
	// WorkerGroup returns the group Nginx workers run as.
	WorkerGroup func(ctx context.Context) string
	// RequireRoot refuses to change anything when not running as root.
	RequireRoot bool
}

// New returns a Lab for this machine.
func New() *Lab {
	instance, _ := config.InstancePath(Instance)
	bin := "nginx"
	if p, err := exec.LookPath("nginx"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			bin = abs
		}
	}
	l := &Lab{InstanceFile: instance, UnitDir: "/etc/systemd/system", Nginx: bin, RequireRoot: true}
	l.Systemctl = func(ctx context.Context, args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	l.TestConfig = func(ctx context.Context, file string) (string, error) {
		out, err := exec.CommandContext(ctx, l.Nginx, "-t", "-c", file).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	l.PortFree = func(port int) bool {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return false
		}
		ln.Close()
		return true
	}
	l.WorkerGroup = func(ctx context.Context) string {
		g, _ := nginx.DetectWorkerGroup(ctx, l.Nginx, "/etc/nginx")
		return g
	}
	return l
}

func (l *Lab) unitFile() string { return filepath.Join(l.UnitDir, Unit+".service") }

func (l *Lab) checkRoot() error {
	if l.RequireRoot && os.Geteuid() != 0 {
		return errors.New("the lab needs root: run gonix with sudo")
	}
	return nil
}

// Load returns the lab's state, or nil when there is no lab.
func (l *Lab) Load() (*State, error) {
	data, err := os.ReadFile(l.InstanceFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(string(data), marker) {
		return nil, fmt.Errorf("%s is an instance the lab did not create; the lab uses the instance name %q", l.InstanceFile, Instance)
	}
	cfg, err := config.Load(l.InstanceFile)
	if err != nil {
		return nil, err
	}
	return readState(cfg.Nginx.ConfigDir)
}

func readState(dir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return nil, fmt.Errorf("reading the lab state: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("reading the lab state: %w", err)
	}
	return &st, nil
}

func (st *State) save() error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(st.Dir, stateFile), append(data, '\n'), 0o644)
}

// Active reports whether the lab's Nginx is running.
func (l *Lab) Active(ctx context.Context) bool {
	out, err := l.Systemctl(ctx, "is-active", Unit)
	return err == nil && out == "active"
}

// Start starts the lab's Nginx.
func (l *Lab) Start(ctx context.Context) error { return l.systemctl(ctx, "start", Unit) }

// Stop stops the lab's Nginx.
func (l *Lab) Stop(ctx context.Context) error { return l.systemctl(ctx, "stop", Unit) }

func (l *Lab) systemctl(ctx context.Context, args ...string) error {
	if err := l.checkRoot(); err != nil {
		return err
	}
	if out, err := l.Systemctl(ctx, args...); err != nil {
		return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// validDir accepts an absolute directory at least two levels deep, which
// is either missing, empty, or a lab already (then it is reset).
func validDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("the lab directory must be an absolute path")
	}
	clean := filepath.Clean(dir)
	if strings.Count(clean, "/") < 2 {
		return fmt.Errorf("%s is too close to /; use a directory of its own, e.g. %s", clean, DefaultDir)
	}
	entries, err := os.ReadDir(clean)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case len(entries) == 0:
		return nil
	}
	if _, err := os.Stat(filepath.Join(clean, stateFile)); err != nil {
		return fmt.Errorf("%s is not empty and is not a lab; choose an empty or new directory", clean)
	}
	return nil
}

// Setup creates (or resets) the lab and starts its Nginx.
func (l *Lab) Setup(ctx context.Context, o Options) (*State, error) {
	if err := l.checkRoot(); err != nil {
		return nil, err
	}
	o.Dir = filepath.Clean(o.Dir)
	if err := validDir(o.Dir); err != nil {
		return nil, err
	}
	if o.HTTPPort < 1 || o.HTTPPort > 65535 || o.HTTPSPort < 1 || o.HTTPSPort > 65535 || o.HTTPPort == o.HTTPSPort {
		return nil, fmt.Errorf("the HTTP and HTTPS ports must be two different ports between 1 and 65535")
	}
	old, err := l.Load()
	if err != nil {
		return nil, err
	}
	if old != nil && old.Dir != o.Dir {
		return nil, fmt.Errorf("a lab already exists in %s; remove it first", old.Dir)
	}
	if old != nil {
		_, _ = l.Systemctl(ctx, "stop", Unit)
	}
	for _, p := range []int{o.HTTPPort, o.HTTPSPort} {
		if !l.PortFree(p) {
			return nil, fmt.Errorf("port %d is in use; choose another one", p)
		}
	}
	if old != nil {
		if err := os.RemoveAll(o.Dir); err != nil {
			return nil, err
		}
	}

	st := &State{Dir: o.Dir, HTTPPort: o.HTTPPort, HTTPSPort: o.HTTPSPort,
		BackendA: "127.0.0.1:19001", BackendB: "127.0.0.1:19002", Created: time.Now()}
	for _, d := range []string{"sites-available", "sites-enabled", "conf.d", "snippets", "streams", "modules", "certs",
		"accesslists", "backups", "logs", "run", "cache", "www/error_pages", "letsencrypt/live", "letsencrypt/renewal", "letsencrypt/archive"} {
		if err := os.MkdirAll(filepath.Join(o.Dir, d), 0o755); err != nil {
			return nil, err
		}
	}
	data := map[string]any{
		"Dir": o.Dir, "User": "", "Nginx": l.Nginx, "Unit": Unit, "Instance": Instance,
		"HTTPPort": o.HTTPPort, "HTTPSPort": o.HTTPSPort, "BackendA": st.BackendA, "BackendB": st.BackendB,
		"Group": l.WorkerGroup(ctx),
	}
	instance, err := render("gonix.yaml.tmpl", data)
	if err != nil {
		return nil, err
	}
	files := map[string]string{
		filepath.Join(o.Dir, "nginx.conf"):               "nginx.conf.tmpl",
		filepath.Join(o.Dir, "mime.types"):               "mime.types.tmpl",
		filepath.Join(o.Dir, "fastcgi_params"):           "fastcgi_params.tmpl",
		filepath.Join(o.Dir, "conf.d/lab-backends.conf"): "backends.conf.tmpl",
		l.unitFile(): "unit.service.tmpl",
	}
	for path, tmpl := range files {
		if err := writeTemplate(path, tmpl, data); err != nil {
			return nil, err
		}
	}
	for _, path := range []string{filepath.Join(o.Dir, "gonix.yaml"), l.InstanceFile} {
		if err := writeFile(path, instance, 0o644); err != nil {
			return nil, err
		}
	}
	if err := st.save(); err != nil {
		return nil, err
	}
	if out, err := l.TestConfig(ctx, filepath.Join(o.Dir, "nginx.conf")); err != nil {
		return st, fmt.Errorf("the lab's nginx -t failed: %v\n%s", err, out)
	}
	if err := l.systemctl(ctx, "daemon-reload"); err != nil {
		return st, err
	}
	if err := l.Start(ctx); err != nil {
		return st, err
	}
	if o.Samples {
		for _, sc := range Scenarios {
			if _, err := l.AddHost(ctx, st, sc.ID, sc.DefaultDomain); err != nil {
				return st, fmt.Errorf("adding the %s test host: %w", sc.ID, err)
			}
		}
	}
	return st, nil
}

// Remove stops the lab and deletes everything Setup created.
func (l *Lab) Remove(ctx context.Context) error {
	if err := l.checkRoot(); err != nil {
		return err
	}
	st, err := l.Load()
	if err != nil {
		// Never delete an instance, unit or directory the lab cannot vouch for.
		return err
	}
	var errs []error
	if _, err := os.Stat(l.unitFile()); err == nil {
		_, _ = l.Systemctl(ctx, "stop", Unit)
		errs = append(errs, os.Remove(l.unitFile()))
		if out, err := l.Systemctl(ctx, "daemon-reload"); err != nil {
			errs = append(errs, fmt.Errorf("systemctl daemon-reload: %v: %s", err, out))
		}
	}
	if st != nil {
		if _, err := os.Stat(filepath.Join(st.Dir, stateFile)); err == nil {
			errs = append(errs, os.RemoveAll(st.Dir))
		}
		errs = append(errs, os.Remove(l.InstanceFile))
	}
	return errors.Join(errs...)
}

// --- templates ---

func render(name string, data any) (string, error) {
	t, err := template.New(filepath.Base(name)).ParseFS(templates.Lab, "lab/"+name)
	if err != nil {
		return "", fmt.Errorf("lab template %s: %w", name, err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("lab template %s: %w", name, err)
	}
	return b.String(), nil
}

func writeTemplate(path, name string, data any) error {
	content, err := render(name, data)
	if err != nil {
		return err
	}
	return writeFile(path, content, 0o644)
}

func writeFile(path, content string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), mode)
}

// Guide is the lab's usage guide, filled in with its state (nil when there
// is no lab yet).
func (l *Lab) Guide(ctx context.Context, st *State) string {
	type hostView struct{ Domain, Title, File, Adopt string }
	data := map[string]any{
		"Dir": DefaultDir, "HTTPPort": DefaultHTTPPort, "HTTPSPort": DefaultHTTPSPort, "Unit": Unit,
		"Instance": Instance, "InstanceFile": l.InstanceFile, "SystemConfig": "/etc/nginx",
		"BackendA": "127.0.0.1:19001", "BackendB": "127.0.0.1:19002", "Active": false, "Hosts": []hostView{},
	}
	if st != nil {
		data["Dir"], data["HTTPPort"], data["HTTPSPort"] = st.Dir, st.HTTPPort, st.HTTPSPort
		data["BackendA"], data["BackendB"], data["Active"] = st.BackendA, st.BackendB, l.Active(ctx)
		var hosts []hostView
		for _, h := range st.Hosts {
			sc, _ := ScenarioByID(h.Scenario)
			hosts = append(hosts, hostView{h.Domain, sc.Title, h.File, sc.Adopt})
		}
		data["Hosts"] = hosts
	}
	out, err := render("guide.txt.tmpl", data)
	if err != nil {
		return err.Error()
	}
	return out
}

// HostFiles lists the test hosts' files, for checks.
func (st *State) HostFiles() []string {
	var out []string
	for _, h := range st.Hosts {
		out = append(out, h.File)
	}
	slices.Sort(out)
	return out
}
