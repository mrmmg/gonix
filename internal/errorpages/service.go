package errorpages

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/nginx"
)

// ConfigTester is satisfied by *nginx.Validator.
type ConfigTester interface {
	Test(ctx context.Context) (nginx.Result, error)
}

// Reloader is satisfied by *system.Service.
type Reloader interface {
	Reload(ctx context.Context) error
}

// Service applies snippet changes safely: a snippet may be included by any
// number of hosts, so every write is followed by `nginx -t`, and the
// previous file content is restored if the test fails.
type Service struct {
	Store    *Store
	Manager  *nginx.Manager // used to find hosts that include a snippet
	Tester   ConfigTester
	Reloader Reloader // may be nil; when nil, changes are validated but not reloaded
	Audit    *audit.Logger
}

// Create writes a brand new snippet.
func (s *Service) Create(ctx context.Context, sn Snippet) error {
	if s.Store.Exists(sn.Name) {
		return fmt.Errorf("an error pages snippet named %q already exists", sn.Name)
	}
	return s.apply(ctx, sn, "error_pages_created")
}

// Update rewrites an existing snippet, e.g. with a different set of codes
// or pages directory. Every host including it picks up the change.
func (s *Service) Update(ctx context.Context, sn Snippet) error {
	if !s.Store.Exists(sn.Name) {
		return fmt.Errorf("error pages snippet %q does not exist", sn.Name)
	}
	return s.apply(ctx, sn, "error_pages_updated")
}

func (s *Service) apply(ctx context.Context, sn Snippet, action string) error {
	content, err := s.Store.Render(sn)
	if err != nil {
		return err
	}
	path := s.Store.Path(sn.Name)
	previous, readErr := os.ReadFile(path)
	existed := readErr == nil

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		s.audit(action, sn.Name, audit.ResultFailure, err)
		return fmt.Errorf("writing snippet %s: %w", path, err)
	}

	restore := func() {
		if existed {
			_ = os.WriteFile(path, previous, 0o644)
		} else {
			_ = os.Remove(path)
		}
	}
	if err := s.testAndReload(ctx, restore); err != nil {
		s.audit(action, sn.Name, audit.ResultFailure, err)
		return err
	}
	s.audit(action, sn.Name, audit.ResultSuccess, nil)
	return nil
}

// Delete removes a snippet. It refuses while any managed host still uses
// it, and restores the file if `nginx -t` fails afterwards (e.g. because a
// hand-written host includes it).
func (s *Service) Delete(ctx context.Context, name string) error {
	users, err := s.UsedBy(name)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return fmt.Errorf("error pages snippet %q is still used by: %s", name, strings.Join(users, ", "))
	}

	path := s.Store.Path(name)
	previous, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading snippet %s: %w", path, err)
	}
	if err := os.Remove(path); err != nil {
		s.audit("error_pages_deleted", name, audit.ResultFailure, err)
		return fmt.Errorf("deleting snippet %s: %w", path, err)
	}
	restore := func() { _ = os.WriteFile(path, previous, 0o644) }
	if err := s.testAndReload(ctx, restore); err != nil {
		s.audit("error_pages_deleted", name, audit.ResultFailure, err)
		return err
	}
	s.audit("error_pages_deleted", name, audit.ResultSuccess, nil)
	return nil
}

// UsedBy returns the server names of managed hosts that include the
// snippet called name.
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
		if h.ErrorPagesSnippet == name {
			users = append(users, h.ServerName)
		}
	}
	return users, nil
}

// testAndReload runs `nginx -t`, calling restore and returning an error if
// it fails, then reloads Nginx.
func (s *Service) testAndReload(ctx context.Context, restore func()) error {
	result, err := s.Tester.Test(ctx)
	if err != nil {
		restore()
		return fmt.Errorf("testing configuration: %w", err)
	}
	if !result.OK {
		restore()
		return fmt.Errorf("nginx configuration test failed, previous snippet restored:\n%s", result.Output)
	}
	if s.Reloader != nil {
		if err := s.Reloader.Reload(ctx); err != nil {
			return fmt.Errorf("reloading nginx: %w", err)
		}
	}
	return nil
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
