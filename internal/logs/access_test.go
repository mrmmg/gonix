package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sampleLine = `203.0.113.7 - - [03/Nov/2026:10:15:01 +0330] "GET /api/users?id=5 HTTP/1.1" 502 157 "https://example.com/" "Mozilla/5.0 (X11; Linux)"`

func TestParseAccessLine(t *testing.T) {
	e, ok := ParseAccessLine(sampleLine)
	if !ok {
		t.Fatal("expected the line to parse")
	}
	if e.IP != "203.0.113.7" || e.Method != "GET" || e.Path != "/api/users?id=5" || e.Status != 502 ||
		e.Bytes != 157 || e.Referer != "https://example.com/" || e.UserAgent != "Mozilla/5.0 (X11; Linux)" {
		t.Errorf("parsed = %+v", e)
	}
	if e.Time.Minute() != 15 || e.Time.Day() != 3 {
		t.Errorf("time = %v", e.Time)
	}

	// "common" format (no referer/agent), dashes and malformed requests.
	if e, ok := ParseAccessLine(`::1 - bob [03/Nov/2026:10:15:01 +0000] "-" 400 - `); !ok || e.Status != 400 || e.IP != "::1" {
		t.Errorf("short line = %+v, %v", e, ok)
	}
	if _, ok := ParseAccessLine("garbage"); ok {
		t.Error("garbage should not parse")
	}
}

func TestTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	var b strings.Builder
	for i := 1; i <= 5000; i++ {
		fmt.Fprintf(&b, "line %d %s\n", i, strings.Repeat("x", 40))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := Tail(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "line 4998 ") || !strings.HasPrefix(lines[2], "line 5000 ") {
		t.Errorf("Tail = %v", lines)
	}
	if lines, _ := Tail(path, 100000); len(lines) != 5000 {
		t.Errorf("Tail(all) returned %d lines", len(lines))
	}

	empty := filepath.Join(t.TempDir(), "empty.log")
	os.WriteFile(empty, nil, 0o644)
	if lines, err := Tail(empty, 10); err != nil || len(lines) != 0 {
		t.Errorf("Tail(empty) = %v, %v", lines, err)
	}
	if _, err := Tail(filepath.Join(t.TempDir(), "missing"), 10); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestFilter(t *testing.T) {
	f, err := ParseFilter("status:5xx ip:203.0.113.7 /api")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Filter{Status: "5xx", IP: "203.0.113.7", Path: "/api"}); f != want {
		t.Errorf("ParseFilter = %+v", f)
	}
	if got, _ := ParseFilter(f.String()); got != f {
		t.Errorf("String round trip = %+v", got)
	}
	e, _ := ParseAccessLine(sampleLine)
	for filter, want := range map[string]bool{
		"": true, "status:5xx": true, "status:502": true, "status:4xx": false, "status:404": false,
		"ip:203.0.113.7": true, "ip:203.0.113.70": false, "path:/api": true, "/admin": false,
	} {
		f, err := ParseFilter(filter)
		if err != nil {
			t.Fatalf("ParseFilter(%q): %v", filter, err)
		}
		if got := f.Match(e); got != want {
			t.Errorf("%q.Match = %v, want %v", filter, got, want)
		}
	}
	for _, bad := range []string{"status:abc", "status:6xx", "host:x"} {
		if _, err := ParseFilter(bad); err == nil {
			t.Errorf("ParseFilter(%q) should fail", bad)
		}
	}
}

func TestSummarize(t *testing.T) {
	var entries []Entry
	for i, raw := range []string{
		`10.0.0.1 - - [03/Nov/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 10 "-" "a"`,
		`10.0.0.1 - - [03/Nov/2026:10:01:00 +0000] "GET /api HTTP/1.1" 502 10 "-" "a"`,
		`10.0.0.2 - - [03/Nov/2026:10:02:00 +0000] "GET /api HTTP/1.1" 502 10 "-" "a"`,
		`10.0.0.1 - - [03/Nov/2026:10:02:00 +0000] "GET /missing HTTP/1.1" 404 10 "-" "a"`,
	} {
		e, ok := ParseAccessLine(raw)
		if !ok {
			t.Fatalf("line %d did not parse", i)
		}
		entries = append(entries, e)
	}
	s := Summarize(entries, 1)
	if s.Total != 4 || s.ByClass["2xx"] != 1 || s.ByClass["5xx"] != 2 || s.ByClass["4xx"] != 1 {
		t.Errorf("counts = %+v", s)
	}
	if !reflect.DeepEqual(s.TopIPs, []Count{{"10.0.0.1", 3}}) || !reflect.DeepEqual(s.Top5xx, []Count{{"/api", 2}}) {
		t.Errorf("tops = %+v / %+v", s.TopIPs, s.Top5xx)
	}
	if s.PerMinute != 2 {
		t.Errorf("PerMinute = %v, want 2", s.PerMinute)
	}
}
