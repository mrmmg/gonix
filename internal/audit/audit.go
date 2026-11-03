// Package audit implements an append-only audit trail of changes made
// through GoNix, independent of the Nginx error/access logs.
package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// Actor says what made the change when it was not the TUI, e.g. "cli"
	// or an AI agent's name (from GONIX_ACTOR).
	Actor string
}

// String renders the entry in the pipe-delimited format documented in the
// project README, e.g.:
//
//	2026-09-15 10:32:11 | user=root | action=host_created | host=example.com | result=success
func (e Entry) String() string {
	s := fmt.Sprintf("%s | user=%s | action=%s | target=%s | result=%s",
		e.Timestamp.Format("2006-01-02 15:04:05"), e.User, e.Action, e.Target, e.Result)
	if e.Actor != "" {
		s += fmt.Sprintf(" | actor=%s", e.Actor)
	}
	if e.Error != "" {
		s += fmt.Sprintf(" | error=%s", e.Error)
	}
	return s
}

// Logger appends entries to a log file on disk. It is intentionally simple;
// rotation is handled by the standard Linux logrotate utility (see
// configs/logrotate.conf, installed by install.sh / `make install`).
type Logger struct {
	path string

	// Actor is recorded on every entry that does not set its own, e.g.
	// "cli". GONIX_ACTOR overrides it.
	Actor string

	// OnSuccess, when set, is called after every successful action is
	// logged, e.g. to commit the configuration to history.
	OnSuccess func(Entry)
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
	if e.Actor == "" {
		e.Actor = l.Actor
		if a := os.Getenv("GONIX_ACTOR"); a != "" {
			e.Actor = a
		}
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("opening audit log: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(e.String() + "\n"); err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	if e.Result == ResultSuccess && l.OnSuccess != nil {
		l.OnSuccess(e)
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

// Path returns the log file path.
func (l *Logger) Path() string { return l.path }

// ParseLine reverses Entry.String. The error field comes last, so an error
// message containing " | " stays intact.
func ParseLine(line string) (Entry, bool) {
	parts := strings.Split(line, " | ")
	if len(parts) < 4 {
		return Entry{}, false
	}
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", parts[0], time.Local)
	if err != nil {
		return Entry{}, false
	}
	e := Entry{Timestamp: ts}
	for i := 1; i < len(parts); i++ {
		key, val, _ := strings.Cut(parts[i], "=")
		switch key {
		case "user":
			e.User = val
		case "action":
			e.Action = val
		case "target", "host":
			e.Target = val
		case "result":
			e.Result = Result(val)
		case "actor":
			e.Actor = val
		case "error":
			e.Error = strings.Join(append([]string{val}, parts[i+1:]...), " | ")
			i = len(parts)
		}
	}
	return e, true
}
