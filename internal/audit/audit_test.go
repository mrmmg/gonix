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

func TestActorHookAndParse(t *testing.T) {
	logger, err := NewLogger(filepath.Join(t.TempDir(), "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	logger.Actor = "cli"
	var hooked []Entry
	logger.OnSuccess = func(e Entry) { hooked = append(hooked, e) }

	ts := time.Date(2026, 9, 15, 10, 32, 11, 0, time.Local)
	logger.Log(Entry{Timestamp: ts, User: "root", Action: "host_created", Target: "example.com", Result: ResultSuccess})
	logger.Log(Entry{Timestamp: ts, User: "root", Action: "host_updated", Target: "example.com", Result: ResultFailure, Error: "a | b"})
	t.Setenv("GONIX_ACTOR", "claude")
	logger.Log(Entry{Timestamp: ts, User: "root", Action: "host_enabled", Target: "example.com", Result: ResultSuccess})

	if len(hooked) != 2 || hooked[0].Actor != "cli" || hooked[1].Actor != "claude" {
		t.Fatalf("hooked = %+v", hooked)
	}
	data, _ := os.ReadFile(logger.Path())
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	e, ok := ParseLine(lines[1])
	if !ok || e.Action != "host_updated" || e.Result != ResultFailure || e.Error != "a | b" || e.Actor != "cli" || !e.Timestamp.Equal(ts) {
		t.Errorf("ParseLine = %+v, %v", e, ok)
	}
	// Lines written before actors existed (and the old host= key) still parse.
	if e, ok := ParseLine("2026-09-15 10:32:11 | user=root | action=host_created | host=example.com"); !ok || e.Target != "example.com" {
		t.Errorf("old line = %+v, %v", e, ok)
	}
	if _, ok := ParseLine("garbage"); ok {
		t.Error("garbage parsed")
	}
}
