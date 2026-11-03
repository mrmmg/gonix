// Package snippet applies changes to standalone Nginx configuration files
// (snippets included by hosts, or http-level files under conf.d) with the
// same guarantees as host changes: every write is followed by `nginx -t`,
// the previous content is restored when the test fails, and every action is
// recorded in the audit log.
package snippet

import (
	"context"
	"fmt"
	"os"
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

// Writer writes, tests and reloads snippet files.
type Writer struct {
	Tester   ConfigTester
	Reloader Reloader // may be nil; when nil, changes are validated but not reloaded
	Audit    *audit.Logger
}

// File is a single file write: Content is written to Path. A nil Content
// removes the file instead.
type File struct {
	Path    string
	Content *string
}

// Write replaces path with content, then validates and reloads.
func (w *Writer) Write(ctx context.Context, action, target, path, content string) error {
	return w.Apply(ctx, action, target, File{Path: path, Content: &content})
}

// Remove deletes path, then validates and reloads. A missing file is not an
// error.
func (w *Writer) Remove(ctx context.Context, action, target, path string) error {
	return w.Apply(ctx, action, target, File{Path: path})
}

// Apply performs every file change, then runs `nginx -t` once. If the test
// fails, every file is restored to what it was before, so related files
// (e.g. an http-level zone and the snippet using it) change together or not
// at all.
func (w *Writer) Apply(ctx context.Context, action, target string, files ...File) error {
	type previous struct {
		path    string
		data    []byte
		existed bool
	}
	var saved []previous
	restore := func() {
		for i := len(saved) - 1; i >= 0; i-- {
			p := saved[i]
			if p.existed {
				_ = os.WriteFile(p.path, p.data, 0o644)
			} else {
				_ = os.Remove(p.path)
			}
		}
	}

	for _, f := range files {
		data, err := os.ReadFile(f.Path)
		saved = append(saved, previous{path: f.Path, data: data, existed: err == nil})
		if f.Content == nil {
			err = os.Remove(f.Path)
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = os.WriteFile(f.Path, []byte(*f.Content), 0o644)
		}
		if err != nil {
			restore()
			err = fmt.Errorf("writing %s: %w", f.Path, err)
			w.audit(action, target, audit.ResultFailure, err)
			return err
		}
	}

	if err := w.testAndReload(ctx, restore); err != nil {
		w.audit(action, target, audit.ResultFailure, err)
		return err
	}
	w.audit(action, target, audit.ResultSuccess, nil)
	return nil
}

func (w *Writer) testAndReload(ctx context.Context, restore func()) error {
	result, err := w.Tester.Test(ctx)
	if err != nil {
		restore()
		return fmt.Errorf("testing configuration: %w", err)
	}
	if !result.OK {
		restore()
		return fmt.Errorf("nginx configuration test failed, previous configuration restored:\n%s", result.Output)
	}
	if w.Reloader != nil {
		if err := w.Reloader.Reload(ctx); err != nil {
			return fmt.Errorf("reloading nginx: %w", err)
		}
	}
	return nil
}

func (w *Writer) audit(action, target string, result audit.Result, err error) {
	if w.Audit == nil {
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
	_ = w.Audit.Log(e)
}
