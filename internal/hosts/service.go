package hosts

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/shiva/nginx-manager/internal/audit"
	"github.com/shiva/nginx-manager/internal/backup"
	"github.com/shiva/nginx-manager/internal/nginx"
)

// ConfigTester is satisfied by *nginx.Validator; it is an interface here so
// tests can substitute a fake `nginx -t` outcome without a real Nginx
// installation.
type ConfigTester interface {
	Test(ctx context.Context) (nginx.Result, error)
}

// Reloader is satisfied by *system.Service; kept as a narrow interface so
// this package does not depend on internal/system directly.
type Reloader interface {
	Reload(ctx context.Context) error
}

// Service implements the safe apply/rollback workflow on top of
// internal/nginx, internal/backup and internal/audit.
type Service struct {
	Manager  *nginx.Manager
	Tester   ConfigTester
	Reloader Reloader // may be nil; when nil, Apply validates but does not reload
	Backups  *backup.Backuper
	Audit    *audit.Logger
}

// ApplyResult describes what happened when applying a host change.
type ApplyResult struct {
	TestOutput string
	RolledBack bool
}

// applyAndValidate writes h to disk (after taking a backup of whatever was
// there before), runs `nginx -t`, and rolls back automatically if the test
// fails. It never leaves the live configuration in a broken state.
func (s *Service) applyAndValidate(ctx context.Context, h nginx.Host, action string) (ApplyResult, error) {
	fileName := h.ServerName
	availablePath := filepath.Join(s.Manager.SitesAvailable, fileName)

	snap, err := s.Backups.Save(fileName, availablePath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("creating backup: %w", err)
	}

	if err := s.Manager.WriteHost(h); err != nil {
		s.audit(action, fileName, audit.ResultFailure, err)
		return ApplyResult{}, fmt.Errorf("writing host configuration: %w", err)
	}

	result, err := s.Tester.Test(ctx)
	if err != nil {
		s.audit(action, fileName, audit.ResultFailure, err)
		return ApplyResult{}, fmt.Errorf("testing configuration: %w", err)
	}

	if !result.OK {
		rolledBack := false
		if snap.Path != "" {
			if rerr := s.Backups.Restore(snap, availablePath); rerr == nil {
				rolledBack = true
			}
		}
		testErr := fmt.Errorf("nginx configuration test failed:\n%s", result.Output)
		s.audit(action, fileName, audit.ResultFailure, testErr)
		return ApplyResult{TestOutput: result.Output, RolledBack: rolledBack}, testErr
	}

	if s.Reloader != nil {
		if err := s.Reloader.Reload(ctx); err != nil {
			s.audit(action, fileName, audit.ResultFailure, err)
			return ApplyResult{TestOutput: result.Output}, fmt.Errorf("reloading nginx: %w", err)
		}
	}

	s.audit(action, fileName, audit.ResultSuccess, nil)
	return ApplyResult{TestOutput: result.Output}, nil
}

func (s *Service) audit(action, target string, result audit.Result, err error) {
	if s.Audit == nil {
		return
	}
	e := audit.Entry{
		Timestamp: time.Now(),
		User:      audit.CurrentUser(),
		Action:    action,
		Target:    target,
		Result:    result,
	}
	if err != nil {
		e.Error = err.Error()
	}
	_ = s.Audit.Log(e)
}

// CreateHost validates and writes a brand new host, applying it safely.
func (s *Service) CreateHost(ctx context.Context, h nginx.Host) (ApplyResult, error) {
	if err := ValidateDomain(h.ServerName); err != nil {
		return ApplyResult{}, err
	}
	if err := h.Validate(); err != nil {
		return ApplyResult{}, err
	}
	return s.applyAndValidate(ctx, h, "host_created")
}

// UpdateHost writes changes to an existing host, applying them safely.
func (s *Service) UpdateHost(ctx context.Context, h nginx.Host) (ApplyResult, error) {
	if err := h.Validate(); err != nil {
		return ApplyResult{}, err
	}
	return s.applyAndValidate(ctx, h, "host_updated")
}

// EnableHost creates the sites-enabled symlink and reloads Nginx.
func (s *Service) EnableHost(ctx context.Context, fileName string) error {
	if err := s.Manager.Enable(fileName); err != nil {
		s.audit("host_enabled", fileName, audit.ResultFailure, err)
		return err
	}
	if err := s.testAndReload(ctx); err != nil {
		_ = s.Manager.Disable(fileName)
		s.audit("host_enabled", fileName, audit.ResultFailure, err)
		return err
	}
	s.audit("host_enabled", fileName, audit.ResultSuccess, nil)
	return nil
}

// DisableHost removes the sites-enabled symlink and reloads Nginx. The
// sites-available file is never touched.
func (s *Service) DisableHost(ctx context.Context, fileName string) error {
	if err := s.Manager.Disable(fileName); err != nil {
		s.audit("host_disabled", fileName, audit.ResultFailure, err)
		return err
	}
	if s.Reloader != nil {
		if err := s.Reloader.Reload(ctx); err != nil {
			s.audit("host_disabled", fileName, audit.ResultFailure, err)
			return err
		}
	}
	s.audit("host_disabled", fileName, audit.ResultSuccess, nil)
	return nil
}

// DeleteHost backs up, removes the host's config file and symlink, and
// reloads Nginx.
func (s *Service) DeleteHost(ctx context.Context, fileName string) error {
	availablePath := filepath.Join(s.Manager.SitesAvailable, fileName)
	if _, err := s.Backups.Save(fileName, availablePath); err != nil {
		return fmt.Errorf("creating backup before delete: %w", err)
	}
	if err := s.Manager.DeleteHost(fileName); err != nil {
		s.audit("host_deleted", fileName, audit.ResultFailure, err)
		return err
	}
	if s.Reloader != nil {
		if err := s.Reloader.Reload(ctx); err != nil {
			s.audit("host_deleted", fileName, audit.ResultFailure, err)
			return err
		}
	}
	s.audit("host_deleted", fileName, audit.ResultSuccess, nil)
	return nil
}

func (s *Service) testAndReload(ctx context.Context) error {
	result, err := s.Tester.Test(ctx)
	if err != nil {
		return fmt.Errorf("testing configuration: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("nginx configuration test failed:\n%s", result.Output)
	}
	if s.Reloader != nil {
		return s.Reloader.Reload(ctx)
	}
	return nil
}
