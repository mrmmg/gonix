// Package audit implements an append-only audit trail of changes made
// through nginx-manager, independent of the Nginx error/access logs.
package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Result describes the outcome of an audited action.
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
)

// Entry is a single audit log record.
type Entry struct {
	Timestamp time.Time
	User      string
	Action    string
	Target    string
	Result    Result
	Error     string
}

// String renders the entry in the pipe-delimited format documented in the
// project README, e.g.:
//
//	2026-09-15 10:32:11 | user=root | action=host_created | host=example.com | result=success
func (e Entry) String() string {
	s := fmt.Sprintf("%s | user=%s | action=%s | target=%s | result=%s",
		e.Timestamp.Format("2006-01-02 15:04:05"), e.User, e.Action, e.Target, e.Result)
	if e.Error != "" {
		s += fmt.Sprintf(" | error=%s", e.Error)
	}
	return s
}

// Logger appends entries to a log file on disk. It is intentionally simple;
// rotation is handled by the standard Linux logrotate utility (see
// scripts/install.sh and configs/logrotate).
type Logger struct {
	path string
}

// NewLogger creates a Logger writing to path, creating parent directories as
// needed.
func NewLogger(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating audit log directory: %w", err)
	}
	return &Logger{path: path}, nil
}

// Log appends a single entry to the audit log. Failures to write the audit
// log are returned but should generally only be surfaced, never block the
// underlying action from having already happened.
func (l *Logger) Log(e Entry) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("opening audit log: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(e.String() + "\n"); err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	return nil
}

// CurrentUser returns the invoking Linux user name, falling back to the
// SUDO_USER / USER environment variables when os/user is unavailable.
func CurrentUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}
