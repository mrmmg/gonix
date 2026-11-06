package metrics

import (
	"slices"
	"sort"

	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/health"
	"github.com/mrmmg/gonix/internal/lint"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/system"
	"github.com/mrmmg/gonix/internal/upstreams"
)

func TestFilter(t *testing.T) {
	f := Filter{Include: []string{"*.example.com"}, Exclude: []string{"staging.*"}}
	cases := []struct {
		names []string
		want  bool
	}{
		{nil, true},
		{[]string{"app.example.com"}, true},
		{[]string{"other.org"}, false},
		{[]string{"other.org", "www.example.com"}, true}, // an alias matches
		{[]string{"staging.example.com"}, false},
	}
	for _, c := range cases {
		if got := f.Match(c.names...); got != c.want {
			t.Errorf("Match(%v) = %v, want %v", c.names, got, c.want)
		}
	}
	if !(Filter{}).Match("anything") {
		t.Error("empty filter should match everything")
	}
}

func TestParseStubStatus(t *testing.T) {
	st, err := parseStubStatus("Active connections: 291 \nserver accepts handled requests\n 16630948 16630948 31070465 \nReading: 6 Writing: 179 Waiting: 106 \n")
	if err != nil {
		t.Fatal(err)
	}
	if st.active != 291 || st.requests != 31070465 || st.writing != 179 || st.waiting != 106 {
		t.Errorf("parsed %+v", st)
	}
	if _, err := parseStubStatus("<html>nope</html>"); err == nil {
		t.Error("expected an error for a page that is not stub_status")
	}
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(text)
}

func TestTailerFollowsRotationAndTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	appendTo(t, path, "old\n")
	tl := newTailer()
	var got []string
	read := func() { tl.poll(path, func(l string) { got = append(got, l) }) }

	read() // first round: starts at the end
	tl.prime()
	if len(got) != 0 {
		t.Fatalf("old lines counted: %v", got)
	}
	appendTo(t, path, "one\ntw")
	read()
	appendTo(t, path, "o\n")
	read()
	if strings.Join(got, ",") != "one,two" {
		t.Fatalf("got %v", got)
	}

	// Rotation: lines written to the old file just before it moved are still read.
	appendTo(t, path, "last-old\n")
	os.Rename(path, path+".1")
	appendTo(t, path, "new-and-longer\n")
	read()
	if strings.Join(got, ",") != "one,two,last-old,new-and-longer" {
		t.Fatalf("after rotation got %v", got)
	}

	// Truncation in place (copytruncate).
	os.Truncate(path, 0)
	appendTo(t, path, "again\n")
	read()
	if got[len(got)-1] != "again" {
		t.Fatalf("after truncation got %v", got)
	}

	// A file created after startup is read from its start.
	late := filepath.Join(filepath.Dir(path), "late.log")
	appendTo(t, late, "first\n")
	var lateGot []string
	tl.poll(late, func(l string) { lateGot = append(lateGot, l) })
	if len(lateGot) != 1 {
		t.Fatalf("late file got %v", lateGot)
	}
}

type fixture struct {
	dir string
	ex  *Exporter
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	hosts := []nginx.ListedHost{
		{Host: nginx.Host{ServerName: "a.example.com", ServerAlias: []string{"www.a.example.com"}, Managed: true, Enabled: true, Mode: nginx.ModeReverseProxy,
			Locations: []nginx.Location{
				{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamHost: "127.0.0.1", UpstreamPort: 8080}},
				{Path: "/app", Proxy: &nginx.ProxyConfig{UpstreamGroup: "app"}},
			}}},
		{Host: nginx.Host{ServerName: "b.test", Managed: true, Mode: nginx.ModeStatic, Maintenance: true}},
	}
	for _, h := range hosts {
		appendTo(t, filepath.Join(dir, h.ServerName+".access.log"), "")
	}
	src := Sources{
		Hosts: func() ([]nginx.ListedHost, error) { return hosts, nil },
		Certs: func() []certificates.Certificate {
			return []certificates.Certificate{{Domain: "a.example.com", SANs: []string{"a.example.com"}, Source: "gonix",
				NotAfter: time.Now().Add(10 * 24 * time.Hour), Status: certificates.StatusExpiringSoon, KeyMatches: true}}
		},
		Probe: func(_ context.Context, addrs []string) map[string]health.Probe {
			out := map[string]health.Probe{}
			for _, a := range addrs {
				out[a] = health.Probe{Err: errors.New("refused")}
				if a == "10.0.0.1:80" {
					out[a] = health.Probe{Duration: time.Millisecond}
				}
			}
			return out
		},
		Groups: func() ([]upstreams.Group, error) {
			return []upstreams.Group{{Name: "app", Servers: []upstreams.Server{{Address: "10.0.0.1:80"}}}}, nil
		},
		Lint: func(context.Context) ([]lint.Finding, error) {
			return []lint.Finding{{Severity: lint.Warning, Target: "a.example.com", Message: "x"}}, nil
		},
		NginxVersion: "1.27.0",
		StubStatus: func(context.Context) (string, error) {
			return "Active connections: 3 \nserver accepts handled requests\n 10 10 20 \nReading: 0 Writing: 1 Waiting: 2 \n", nil
		},
		NginxStatus: func(context.Context) (system.Status, error) { return system.Status{Active: true, Enabled: true}, nil },
		NginxTest:   func(context.Context) (bool, error) { return true, nil },
		Processes: func(context.Context) (system.ProcessInfo, error) {
			return system.ProcessInfo{MasterPID: 1, WorkerPIDs: []int{2, 3}, WorkerCount: 2}, nil
		},
		RSS:    func(int) (uint64, error) { return 1000, nil },
		Uptime: func(int) (time.Duration, error) { return time.Minute, nil },
		LogDir: dir, AuditFile: filepath.Join(dir, "audit.log"),
	}
	ex := New(Options{Instance: "staging", Version: "test", CacheTTL: time.Minute}, src)
	t.Cleanup(ex.Close)
	return &fixture{dir, ex}
}

func scrape(t *testing.T, ex *Exporter, query string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	ex.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	return rec.Body.String()
}

func TestScrapeEndToEnd(t *testing.T) {
	f := newFixture(t)
	scrape(t, f.ex, "") // first scrape: logs are followed from their end

	access := filepath.Join(f.dir, "a.example.com.access.log")
	line := func(status int) string {
		return fmt.Sprintf(`1.2.3.4 - - [06/Nov/2026:10:00:00 +0000] "GET / HTTP/1.1" %d 100 "-" "curl"`+"\n", status)
	}
	appendTo(t, access, line(200)+line(200)+line(502))
	appendTo(t, filepath.Join(f.dir, "a.example.com.error.log"), "2026/11/06 10:00:00 [error] 1#1: boom\n")
	appendTo(t, filepath.Join(f.dir, "audit.log"), "2026-11-06 10:00:00 | user=root | action=host_created | target=a | result=failure\n")

	out := scrape(t, f.ex, "")
	for _, want := range []string{
		`gonix_http_requests_total{code="200",gonix_instance="staging",host="a.example.com"} 2`,
		`gonix_http_requests_total{code="502",gonix_instance="staging",host="a.example.com"} 1`,
		`gonix_http_response_bytes_total{gonix_instance="staging",host="a.example.com"} 300`,
		`gonix_error_log_entries_total{gonix_instance="staging",host="a.example.com",level="error"} 1`,
		`gonix_audit_events_total{action="host_created",gonix_instance="staging",result="failure"} 1`,
		`gonix_hosts{gonix_instance="staging",state="maintenance"} 1`,
		`gonix_host_maintenance{gonix_instance="staging",host="b.test"} 1`,
		`gonix_upstream_up{gonix_instance="staging",group="",host="a.example.com",upstream="127.0.0.1:8080"} 0`,
		`gonix_certificate_due_for_renewal{domain="a.example.com",gonix_instance="staging",source="gonix"} 1`,
		`gonix_nginx_up{gonix_instance="staging"} 1`,
		`gonix_nginx_processes{gonix_instance="staging",type="worker"} 2`,
		`gonix_nginx_config_valid{gonix_instance="staging"} 1`,
		`gonix_build_info{gonix_instance="staging",version="test"} 1`,
		`gonix_exporter_collector_success{collector="traffic",gonix_instance="staging"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestScrapeFiltersByHostAndCollector(t *testing.T) {
	f := newFixture(t)

	out := scrape(t, f.ex, "?host=b.test")
	if strings.Contains(out, `host="a.example.com"`) || strings.Contains(out, `domain="a.example.com"`) {
		t.Error("host=b.test still shows a.example.com")
	}
	if !strings.Contains(out, `host="b.test"`) {
		t.Error("host=b.test shows nothing about b.test")
	}
	if !strings.Contains(out, "gonix_nginx_up") {
		t.Error("series that are not about a host must stay")
	}

	out = scrape(t, f.ex, "?exclude_host=*.example.com")
	if strings.Contains(out, `host="a.example.com"`) {
		t.Error("exclude_host did not remove a.example.com")
	}

	out = scrape(t, f.ex, "?collect[]=certificates")
	if strings.Contains(out, "gonix_nginx_up") || !strings.Contains(out, "gonix_certificate_remaining_seconds") {
		t.Error("collect[]=certificates should only run that collector")
	}

	rec := httptest.NewRecorder()
	f.ex.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics?collect[]=nope", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown collector: status %d, want 400", rec.Code)
	}
}

func TestConfiguredFilterAndCollectors(t *testing.T) {
	f := newFixture(t)
	ex := New(Options{Instance: "x", Filter: Filter{Exclude: []string{"b.test"}}, Collectors: []string{"gonix"}, CacheTTL: time.Minute}, f.ex.src)
	defer ex.Close()
	out := scrape(t, ex, "")
	if strings.Contains(out, `host="b.test"`) {
		t.Error("configured exclude ignored")
	}
	if strings.Contains(out, "gonix_nginx_up") || strings.Contains(out, "gonix_certificate") {
		t.Error("disabled collectors still run")
	}
}

func TestServeAndShutdown(t *testing.T) {
	f := newFixture(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.ex.Serve(ctx, addr) }()

	var body string
	for i := 0; i < 50; i++ {
		if resp, err := http.Get("http://" + addr + "/metrics?collect[]=certificates"); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			body = string(b)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(body, "gonix_certificate_remaining_seconds") {
		t.Errorf("no metrics served:\n%s", body)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Serve returned %v after shutdown", err)
	}
}

// The definitions in defs.go are what the dashboard generator and its tests
// rely on, so what is really served must match them: every metric defined,
// with the defined type and labels, and nothing undefined.
func TestDefinitionsMatchWhatIsServed(t *testing.T) {
	f := newFixture(t)
	scrape(t, f.ex, "")
	appendTo(t, filepath.Join(f.dir, "a.example.com.access.log"),
		`1.2.3.4 - - [06/Nov/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 5 "-" "curl"`+"\n")
	appendTo(t, filepath.Join(f.dir, "a.example.com.error.log"), "2026/11/06 10:00:00 [error] 1#1: boom\n")
	appendTo(t, filepath.Join(f.dir, "audit.log"), "2026-11-06 10:00:00 | user=root | action=host_created | target=a | result=success\n")

	reg, err := f.ex.Registry(context.Background(), Filter{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, mf := range families {
		def, ok := Lookup(mf.GetName())
		if !ok {
			t.Errorf("%s is served but not defined in defs.go", mf.GetName())
			continue
		}
		seen[def.Name] = true
		wantType := map[Type]dto.MetricType{Gauge: dto.MetricType_GAUGE, Counter: dto.MetricType_COUNTER}[def.Type]
		if mf.GetType() != wantType {
			t.Errorf("%s is a %v, defined as %s", def.Name, mf.GetType(), def.Type)
		}
		want := append([]string{LabelInstance}, def.Labels...)
		sort.Strings(want)
		for _, m := range mf.Metric {
			var got []string
			for _, l := range m.Label {
				got = append(got, l.GetName())
			}
			sort.Strings(got)
			if !slices.Equal(got, want) {
				t.Errorf("%s has labels %v, defined as %v", def.Name, got, want)
				break
			}
		}
	}
	for _, d := range Defs {
		if !seen[d.Name] {
			t.Errorf("%s is defined but the fixture does not serve it (extend the fixture, or drop the definition)", d.Name)
		}
	}
}
