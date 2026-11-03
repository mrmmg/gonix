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
	"path/filepath"
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
	// DryRun, when set, records every change instead of making it.
	DryRun *Plan
}

// Change is one change recorded in dry-run mode: a file write or removal,
// or an action described by Note (e.g. enabling a host).
type Change struct {
	Path    string
	Old     string // content before; "" when the file did not exist
	New     string // content after; "" when removed
	Existed bool
	Removed bool
	Note    string
}

// Plan collects the changes a dry run would have made.
type Plan struct {
	Changes []Change
}

// RecordFile records writing content (or removing, when content is nil)
// at path.
func (p *Plan) RecordFile(path string, content *string) {
	c := Change{Path: path}
	if data, err := os.ReadFile(path); err == nil {
		c.Old, c.Existed = string(data), true
	}
	if content == nil {
		if !c.Existed {
			return
		}
		c.Removed = true
	} else {
		c.New = *content
		if c.Existed && c.Old == c.New {
			return
		}
	}
	p.Changes = append(p.Changes, c)
}

// RecordNote records an action that is not a file write.
func (p *Plan) RecordNote(note string) {
	p.Changes = append(p.Changes, Change{Note: note})
}

// File is a single file write: Content is written to Path. A nil Content
// removes the file instead.
type File struct {
	Path    string
	Content *string
	Mode    os.FileMode // permissions of a newly created file; 0 means 0644
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
	if w.DryRun != nil {
		for _, f := range files {
			w.DryRun.RecordFile(f.Path, f.Content)
		}
		return nil
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
			mode := f.Mode
			if mode == 0 {
				mode = 0o644
			}
			if err = os.MkdirAll(filepath.Dir(f.Path), 0o755); err == nil {
				err = os.WriteFile(f.Path, []byte(*f.Content), mode)
			}
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
