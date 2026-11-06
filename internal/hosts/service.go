package hosts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/backup"
	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
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
	// DryRun, when set, records every change instead of making it.
	DryRun *snippet.Plan
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
	fileName := h.ConfigFile()
	availablePath := filepath.Join(s.Manager.SitesAvailable, fileName)

	if s.DryRun != nil {
		content, err := s.Manager.Renderer.Render(h)
		if err != nil {
			return ApplyResult{}, err
		}
		s.DryRun.RecordFile(availablePath, &content)
		return ApplyResult{}, nil
	}

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
	if h.Adopted != nil {
		return ApplyResult{}, fmt.Errorf("host %s is adopted from an existing file and cannot be created", h.ServerName)
	}
	if err := ValidateDomain(h.ServerName); err != nil {
		return ApplyResult{}, err
	}
	if err := validateAliases(h.ServerAlias); err != nil {
		return ApplyResult{}, err
	}
	if err := h.Validate(); err != nil {
		return ApplyResult{}, err
	}
	return s.applyAndValidate(ctx, h, "host_created")
}

// UpdateHost writes changes to an existing host, applying them safely.
func (s *Service) UpdateHost(ctx context.Context, h nginx.Host) (ApplyResult, error) {
	aliases := h.ServerAlias
	if h.Adopted != nil {
		// A hand-written file may use names GoNix would not create
		// (wildcards, regular expressions); only new ones are checked.
		existing := nginx.AdoptedServerNames(h.Adopted.Source)
		aliases = slices.DeleteFunc(slices.Clone(aliases), func(a string) bool { return slices.Contains(existing, a) })
	}
	if err := validateAliases(aliases); err != nil {
		return ApplyResult{}, err
	}
	if err := h.Validate(); err != nil {
		return ApplyResult{}, err
	}
	return s.applyAndValidate(ctx, h, "host_updated")
}

// EnableHost creates the sites-enabled symlink and reloads Nginx.
func (s *Service) EnableHost(ctx context.Context, fileName string) error {
	if s.DryRun != nil {
		s.DryRun.RecordNote("enable host " + fileName)
		return nil
	}
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
	if s.DryRun != nil {
		s.DryRun.RecordNote("disable host " + fileName)
		return nil
	}
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
	if s.DryRun != nil {
		s.DryRun.RecordNote("disable host " + fileName)
		s.DryRun.RecordFile(availablePath, nil)
		return nil
	}
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

func validateAliases(aliases []string) error {
	for _, a := range aliases {
		if err := ValidateDomain(a); err != nil {
			return fmt.Errorf("additional server name: %w", err)
		}
	}
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

// AdoptHost marks the hand-written file fileName as managed by GoNix (see
// nginx.AdoptContent). Only a comment is added, so Nginx keeps serving the
// same configuration; the file is still checked with nginx -t, and
// restored if the check fails. No reload is needed.
func (s *Service) AdoptHost(ctx context.Context, fileName string) (nginx.AdoptionReport, error) {
	content, err := s.Manager.RawConfig(fileName)
	if err != nil {
		return nginx.AdoptionReport{}, err
	}
	switch {
	case nginx.IsAdopted(content):
		return nginx.AdoptionReport{}, fmt.Errorf("%s is already adopted", fileName)
	case strings.Contains(content, nginx.ManagedMarker):
		return nginx.AdoptionReport{}, fmt.Errorf("%s was created by GoNix and is already managed", fileName)
	}
	rep, err := s.Manager.Renderer.InspectAdoption(fileName, content)
	if err != nil {
		return rep, fmt.Errorf("%s cannot be adopted: %w", fileName, err)
	}
	if err := ValidateDomain(rep.Host.ServerName); err != nil {
		return rep, fmt.Errorf("%s cannot be adopted: the first name in server_name must be a plain domain (%v); put one first", fileName, err)
	}
	return rep, s.replaceFile(ctx, fileName, content, nginx.AdoptContent(content), "host_adopted")
}

// ReleaseHost undoes AdoptHost: the file goes back to being unmanaged,
// keeping every change GoNix made while it managed it.
func (s *Service) ReleaseHost(ctx context.Context, fileName string) error {
	content, err := s.Manager.RawConfig(fileName)
	if err != nil {
		return err
	}
	released, err := nginx.ReleaseContent(content)
	if err != nil {
		return fmt.Errorf("%s: %w", fileName, err)
	}
	return s.replaceFile(ctx, fileName, content, released, "host_released")
}

// replaceFile swaps a host file's content for one that differs only in
// comments: backup, write, nginx -t, and restore on failure.
func (s *Service) replaceFile(ctx context.Context, fileName, old, content, action string) error {
	path := filepath.Join(s.Manager.SitesAvailable, fileName)
	if s.DryRun != nil {
		s.DryRun.RecordFile(path, &content)
		return nil
	}
	cur, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(cur) != old {
		return fmt.Errorf("%s changed while it was being read; try again", path)
	}
	snap, err := s.Backups.Save(fileName, path)
	if err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}
	if err := s.Manager.WriteFile(fileName, content); err != nil {
		s.audit(action, fileName, audit.ResultFailure, err)
		return err
	}
	result, err := s.Tester.Test(ctx)
	if err == nil && !result.OK {
		err = fmt.Errorf("nginx configuration test failed:\n%s", result.Output)
	}
	if err != nil {
		if rerr := s.Backups.Restore(snap, path); rerr != nil {
			err = fmt.Errorf("%w (restoring the previous file also failed: %v)", err, rerr)
		}
		s.audit(action, fileName, audit.ResultFailure, err)
		return err
	}
	s.audit(action, fileName, audit.ResultSuccess, nil)
	return nil
}

// AdoptionNotes describes, one line each, what adopting a file means:
// where its certificate comes from and who renews it, and what GoNix
// leaves as written.
func AdoptionNotes(rep nginx.AdoptionReport) []string {
	var out []string
	h := rep.Host
	if h.SSL.Enabled {
		switch l, ok := certificates.CertbotLineage(h.SSL.CertFile); {
		case h.SSL.CertFile == "":
			out = append(out, "certificate: set at http level, outside this file")
		case ok:
			auth := l.Authenticator
			if auth == "" {
				auth = "unknown"
			}
			out = append(out, fmt.Sprintf("certificate: %s, renewed by certbot (authenticator %s)", h.SSL.CertFile, auth))
			for _, n := range l.Notes() {
				out = append(out, "  - "+n)
			}
		default:
			out = append(out, "certificate: "+h.SSL.CertFile)
		}
		if rep.Certificates > 1 {
			out = append(out, fmt.Sprintf("the server block has %d certificates; GoNix will not switch it to another one", rep.Certificates))
		}
	}
	if rep.HTTPBlock {
		out = append(out, "a separate server block redirects plain HTTP to HTTPS; it is kept, and follows changes to the domain names")
	}
	if len(rep.Kept) > 0 {
		out = append(out, "kept as written: "+strings.Join(rep.Kept, "; "))
	}
	if len(rep.Custom) > 0 {
		out = append(out, "custom locations (kept as written): "+strings.Join(rep.Custom, ", "))
	}
	return out
}
