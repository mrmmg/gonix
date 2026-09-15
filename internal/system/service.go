// Package system controls and inspects the Nginx system service (via
// systemctl) and its running processes, and provides a lightweight resource
// usage overview. It shells out to standard Linux utilities where no good
// Go-native alternative exists.
package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CommandRunner executes an external command and returns its combined
// stdout+stderr output. It is an interface so tests can substitute a fake
// systemctl/nginx without requiring a real Linux service manager.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (output string, err error)
}

// ExecRunner is the real CommandRunner implementation, backed by os/exec.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Service manages the Nginx systemd unit.
type Service struct {
	Runner     CommandRunner
	UnitName   string // typically "nginx"
	BinaryPath string // typically "nginx", used for `nginx -t`
}

// NewService returns a Service using the real system command runner.
func NewService(binaryPath string) *Service {
	return &Service{Runner: ExecRunner{}, UnitName: "nginx", BinaryPath: binaryPath}
}

// Status describes the current state of the Nginx service.
type Status struct {
	Active     bool // systemctl is-active
	ActiveRaw  string
	Enabled    bool // systemctl is-enabled (starts on boot)
	EnabledRaw string
}

func (s *Service) systemctl(ctx context.Context, args ...string) (string, error) {
	full := append([]string{}, args...)
	return s.Runner.Run(ctx, "systemctl", full...)
}

// GetStatus queries systemd for the current run state of the Nginx unit.
func (s *Service) GetStatus(ctx context.Context) (Status, error) {
	var st Status

	out, _ := s.systemctl(ctx, "is-active", s.UnitName)
	st.ActiveRaw = strings.TrimSpace(out)
	st.Active = st.ActiveRaw == "active"

	out, _ = s.systemctl(ctx, "is-enabled", s.UnitName)
	st.EnabledRaw = strings.TrimSpace(out)
	st.Enabled = st.EnabledRaw == "enabled"

	return st, nil
}

// Start starts the Nginx service.
func (s *Service) Start(ctx context.Context) error {
	out, err := s.systemctl(ctx, "start", s.UnitName)
	if err != nil {
		return fmt.Errorf("starting nginx: %w: %s", err, out)
	}
	return nil
}

// Stop stops the Nginx service.
func (s *Service) Stop(ctx context.Context) error {
	out, err := s.systemctl(ctx, "stop", s.UnitName)
	if err != nil {
		return fmt.Errorf("stopping nginx: %w: %s", err, out)
	}
	return nil
}

// Restart restarts the Nginx service unconditionally.
func (s *Service) Restart(ctx context.Context) error {
	out, err := s.systemctl(ctx, "restart", s.UnitName)
	if err != nil {
		return fmt.Errorf("restarting nginx: %w: %s", err, out)
	}
	return nil
}

// Reload asks Nginx to reload its configuration without dropping
// connections. Callers are expected to have already validated the
// configuration with `nginx -t`.
func (s *Service) Reload(ctx context.Context) error {
	out, err := s.systemctl(ctx, "reload", s.UnitName)
	if err != nil {
		return fmt.Errorf("reloading nginx: %w: %s", err, out)
	}
	return nil
}
