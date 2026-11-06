// Package exporter controls GoNix's Prometheus exporter as a service: its
// systemd unit, its settings and the stub_status endpoint it reads. GoNix
// owns all of it, so a setting changed in GoNix (the port, the filters) is
// applied everywhere at once; the unit itself never contains a setting, it
// just starts "gonix exporter run" with the instance's configuration file.
package exporter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/snippet"
)

// StubStatusFile is the http-level snippet serving stub_status.
const StubStatusFile = "gonix-stub-status.conf"

// Service manages one instance's exporter.
type Service struct {
	// Config is the live configuration; Apply keeps its Monitoring section current.
	Config     *config.Config
	ConfigPath string
	Binary     string // the gonix executable the unit starts
	UnitDir    string // normally /etc/systemd/system
	// Systemctl runs systemctl. Tests replace it.
	Systemctl func(ctx context.Context, args ...string) (string, error)
	// Writer writes the stub_status snippet (tested with nginx -t, then reloaded).
	Writer *snippet.Writer
}

// New returns a Service for cfg, using the running executable and systemd.
func New(cfg *config.Config, cfgPath string, w *snippet.Writer) *Service {
	bin, err := os.Executable()
	if err != nil {
		bin = "/usr/local/bin/gonix"
	}
	return &Service{Config: cfg, ConfigPath: cfgPath, Binary: bin, UnitDir: "/etc/systemd/system", Writer: w,
		Systemctl: func(ctx context.Context, args ...string) (string, error) {
			out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
			return strings.TrimSpace(string(out)), err
		}}
}

// UnitName is the systemd unit of this instance's exporter.
func (s *Service) UnitName() string {
	if s.Config.Instance != "" {
		return "gonix-exporter-" + s.Config.Instance
	}
	return "gonix-exporter"
}

func (s *Service) unitPath() string { return filepath.Join(s.UnitDir, s.UnitName()+".service") }

// StubStatusPath is where the stub_status snippet lives.
func (s *Service) StubStatusPath() string {
	return filepath.Join(s.Config.Nginx.HTTPSnippetsDir, StubStatusFile)
}

// StubStatusURL is the page the exporter reads when stub_status is on, else "".
func (s *Service) StubStatusURL() string {
	if !s.Config.Monitoring.StubStatus.Enabled {
		return ""
	}
	return "http://" + s.Config.Monitoring.StubStatus.Listen + "/stub_status"
}

var unitTmpl = template.Must(template.New("unit").Parse(`# Managed by GoNix{{ if .Instance }} (instance {{ .Instance }}){{ end }}. Change settings with "gonix exporter set", not here.
[Unit]
Description=GoNix Prometheus exporter{{ if .Instance }} (instance {{ .Instance }}){{ end }}
After=network.target

[Service]
ExecStart={{ .Binary }} exporter run --config {{ .Config }}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`))

// Unit renders the systemd unit.
func (s *Service) Unit() string {
	var b bytes.Buffer
	_ = unitTmpl.Execute(&b, map[string]string{"Instance": s.Config.Instance, "Binary": s.Binary, "Config": s.ConfigPath})
	return b.String()
}

// ensureUnit writes the unit if it is missing or differs, and tells systemd.
func (s *Service) ensureUnit(ctx context.Context) error {
	want := s.Unit()
	if have, err := os.ReadFile(s.unitPath()); err == nil && string(have) == want {
		return nil
	}
	if err := os.MkdirAll(s.UnitDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.unitPath(), []byte(want), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", s.unitPath(), err)
	}
	return s.systemctl(ctx, "daemon-reload")
}

func (s *Service) systemctl(ctx context.Context, args ...string) error {
	out, err := s.Systemctl(ctx, args...)
	if err != nil {
		return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// Status describes the exporter service.
type Status struct {
	Unit      string `json:"unit"`
	Installed bool   `json:"installed"`
	UnitStale bool   `json:"unit_stale"` // the unit differs from what GoNix would write now
	Active    bool   `json:"running"`
	State     string `json:"state"`
	Enabled   bool   `json:"enabled_at_boot"`
	Listen    string `json:"listen"`
	URL       string `json:"url"`
}

// Status asks systemd about the exporter.
func (s *Service) Status(ctx context.Context) Status {
	st := Status{Unit: s.UnitName(), Listen: s.Config.Monitoring.Listen, URL: "http://" + s.Config.Monitoring.Listen + "/metrics"}
	have, err := os.ReadFile(s.unitPath())
	st.Installed = err == nil
	st.UnitStale = st.Installed && string(have) != s.Unit()
	out, _ := s.Systemctl(ctx, "is-active", s.UnitName())
	st.State = strings.TrimSpace(out)
	st.Active = st.State == "active"
	out, _ = s.Systemctl(ctx, "is-enabled", s.UnitName())
	st.Enabled = strings.TrimSpace(out) == "enabled"
	return st
}

// Enable installs the unit, then starts the exporter now and at every boot.
func (s *Service) Enable(ctx context.Context) error {
	if !s.Status(ctx).Active {
		if err := portFree(s.Config.Monitoring.Listen); err != nil {
			return err
		}
	}
	if err := s.ensureUnit(ctx); err != nil {
		return err
	}
	return s.systemctl(ctx, "enable", "--now", s.UnitName())
}

// Disable stops the exporter and keeps it from starting at boot. The unit and
// the settings stay.
func (s *Service) Disable(ctx context.Context) error {
	if _, err := os.Stat(s.unitPath()); err != nil {
		return nil
	}
	return s.systemctl(ctx, "disable", "--now", s.UnitName())
}

// Start starts the exporter now (without enabling it at boot).
func (s *Service) Start(ctx context.Context) error {
	if s.Status(ctx).Active {
		return nil
	}
	if err := portFree(s.Config.Monitoring.Listen); err != nil {
		return err
	}
	if err := s.ensureUnit(ctx); err != nil {
		return err
	}
	return s.systemctl(ctx, "start", s.UnitName())
}

// Stop stops the exporter.
func (s *Service) Stop(ctx context.Context) error { return s.systemctl(ctx, "stop", s.UnitName()) }

// Restart restarts the exporter, rewriting an outdated unit first.
func (s *Service) Restart(ctx context.Context) error {
	if err := s.ensureUnit(ctx); err != nil {
		return err
	}
	return s.systemctl(ctx, "restart", s.UnitName())
}

// Uninstall stops the exporter and removes its unit; the settings stay.
func (s *Service) Uninstall(ctx context.Context) error {
	if err := s.Disable(ctx); err != nil {
		return err
	}
	if err := os.Remove(s.unitPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.systemctl(ctx, "daemon-reload")
}

// Apply validates and saves new settings, applies the stub_status endpoint,
// and restarts the exporter if it is running so the change takes effect.
// Nothing is saved if the settings are invalid or Nginx refuses the endpoint.
func (s *Service) Apply(ctx context.Context, m config.MonitoringConfig) error {
	if err := m.Validate(); err != nil {
		return err
	}
	old := s.Config.Monitoring
	if m.Listen != old.Listen && !s.Status(ctx).Active {
		if err := portFree(m.Listen); err != nil {
			return err
		}
	}
	if m.StubStatus != old.StubStatus {
		if err := s.applyStubStatus(ctx, m.StubStatus); err != nil {
			return err
		}
	}
	if err := config.SaveMonitoring(s.ConfigPath, m); err != nil {
		return err
	}
	s.Config.Monitoring = m
	if s.Status(ctx).Active {
		return s.Restart(ctx)
	}
	return nil
}

func (s *Service) applyStubStatus(ctx context.Context, c config.StubStatusConfig) error {
	if s.Writer == nil {
		return fmt.Errorf("cannot change the stub_status endpoint here")
	}
	if !c.Enabled {
		return s.Writer.Remove(ctx, "stub_status_removed", "stub_status", s.StubStatusPath())
	}
	return s.Writer.Write(ctx, "stub_status_configured", "stub_status", s.StubStatusPath(), StubStatusSnippet(c.Listen))
}

// StubStatusSnippet is a server block answering stub_status on listen, for
// local clients only.
func StubStatusSnippet(listen string) string {
	return fmt.Sprintf(`# Managed by GoNix: Nginx connection counters for the Prometheus exporter.
server {
    listen %s;
    server_name _;
    access_log off;
    allow 127.0.0.1;
    allow ::1;
    deny all;

    location = /stub_status {
        stub_status;
    }
}
`, listen)
}

// portFree reports an error if something already listens on addr.
func portFree(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot use %s: %v (pick another address with 'gonix exporter set --listen')", addr, err)
	}
	return ln.Close()
}
