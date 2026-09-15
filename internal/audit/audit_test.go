package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogAppendsEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	logger, err := NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	ts := time.Date(2026, 9, 15, 10, 32, 11, 0, time.UTC)
	if err := logger.Log(Entry{Timestamp: ts, User: "root", Action: "host_created", Target: "example.com", Result: ResultSuccess}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := logger.Log(Entry{Timestamp: ts, User: "root", Action: "ssl_updated", Target: "example.com", Result: ResultFailure, Error: "nginx -t failed"}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading audit log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "action=host_created") || !strings.Contains(lines[0], "result=success") {
		t.Errorf("unexpected first line: %s", lines[0])
	}
	if !strings.Contains(lines[1], "error=nginx -t failed") {
		t.Errorf("unexpected second line: %s", lines[1])
	}
}

func TestEntryStringFormat(t *testing.T) {
	e := Entry{
		Timestamp: time.Date(2026, 9, 15, 10, 32, 11, 0, time.UTC),
		User:      "root", Action: "host_enabled", Target: "example.com", Result: ResultSuccess,
	}
	want := "2026-09-15 10:32:11 | user=root | action=host_enabled | target=example.com | result=success"
	if got := e.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
