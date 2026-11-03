package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/accesslist"
	"github.com/mrmmg/gonix/internal/app"
	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/backup"
	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/errorpages"
	"github.com/mrmmg/gonix/internal/geoblock"
	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/maintenance"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/presets"
	"github.com/mrmmg/gonix/internal/ratelimit"
	"github.com/mrmmg/gonix/internal/realip"
	"github.com/mrmmg/gonix/internal/snippet"
	"github.com/mrmmg/gonix/internal/streams"
	"github.com/mrmmg/gonix/internal/system"
	"github.com/mrmmg/gonix/internal/upstreams"
)

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) {
	return nginx.Result{OK: true, Output: "test is successful"}, nil
}

// newServices wires every service against temporary directories with a
// fake nginx -t, like the real app.Build does against /etc.
func newServices(t *testing.T) *app.Services {
	t.Helper()
	root := t.TempDir()
	cfg := config.Default()
	dir := func(name string) string {
		d := filepath.Join(root, name)
		os.MkdirAll(d, 0o755)
		return d
	}
	cfg.Nginx.ConfigDir = dir("nginx")
	cfg.Nginx.SitesAvailable, cfg.Nginx.SitesEnabled = dir("nginx/sites-available"), dir("nginx/sites-enabled")
	cfg.Nginx.HTTPSnippetsDir, cfg.Nginx.StreamsDir = dir("nginx/conf.d"), dir("nginx/streams")
	cfg.Nginx.MainIncludeDir, cfg.Nginx.CacheDir = dir("nginx/modules"), dir("cache")
	cfg.ErrorPages.SnippetsDirectory, cfg.AccessLists.Directory = dir("nginx/snippets"), dir("accesslists")
	cfg.Certificates.Directory, cfg.Logs.NginxDirectory = dir("certs"), dir("log")
	os.WriteFile(filepath.Join(cfg.Nginx.ConfigDir, "nginx.conf"), []byte("http { include "+cfg.Nginx.HTTPSnippetsDir+"/*.conf; }\n"), 0o644)

	r, err := nginx.NewRenderer(cfg.Logs.NginxDirectory, cfg.Logs.NginxDirectory, cfg.AccessLists.Directory, cfg.ErrorPages.SnippetsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	mgr := nginx.NewManager(cfg.Nginx.SitesAvailable, cfg.Nginx.SitesEnabled, r)
	auditLog, _ := audit.NewLogger(filepath.Join(root, "audit.log"))
	writer := &snippet.Writer{Tester: okTester{}, Audit: auditLog}
	hostSvc := &hosts.Service{Manager: mgr, Tester: okTester{}, Backups: backup.New(dir("backups"), 5), Audit: auditLog}
	ipStore, _ := iplist.NewStore(cfg.ErrorPages.SnippetsDirectory)
	epStore, _ := errorpages.NewStore(cfg.ErrorPages.SnippetsDirectory)
	alStore, _ := accesslist.NewStore(cfg.AccessLists.Directory, "")
	streamSvc := &streams.Service{Dir: cfg.Nginx.StreamsDir, MainIncludeDir: cfg.Nginx.MainIncludeDir, ConfigDir: cfg.Nginx.ConfigDir,
		SnippetsDir: cfg.ErrorPages.SnippetsDirectory, Writer: writer}
	return &app.Services{
		Config: cfg, Manager: mgr, HostService: hostSvc, AccessLists: alStore,
		ErrorPages:  &errorpages.Service{Store: epStore, Manager: mgr, Tester: okTester{}, Audit: auditLog},
		IPLists:     &iplist.Service{Store: ipStore, Manager: mgr, Writer: writer, OtherUsers: streamSvc.UsingIPList},
		RealIP:      &realip.Service{Dir: cfg.Nginx.HTTPSnippetsDir, Writer: writer},
		Presets:     &presets.Service{SnippetsDir: cfg.ErrorPages.SnippetsDirectory, HTTPDir: cfg.Nginx.HTTPSnippetsDir, CacheDir: cfg.Nginx.CacheDir, Writer: writer},
		Maintenance: &maintenance.Service{HTTPDir: cfg.Nginx.HTTPSnippetsDir, Hosts: hostSvc, Writer: writer},
		Upstreams:   &upstreams.Service{Dir: cfg.Nginx.HTTPSnippetsDir, Manager: mgr, Writer: writer},
		Streams:     streamSvc,
		GeoBlock: &geoblock.Service{HTTPDir: cfg.Nginx.HTTPSnippetsDir, SnippetsDir: cfg.ErrorPages.SnippetsDirectory,
			Database: cfg.GeoIP.Database, Manager: mgr, Writer: writer},
		RateLimits: &ratelimit.Service{SnippetsDir: cfg.ErrorPages.SnippetsDirectory, HTTPDir: cfg.Nginx.HTTPSnippetsDir, Manager: mgr, Writer: writer},
		SystemSvc:  system.NewService("nginx"),
		Audit:      auditLog,
		Snippets:   writer,
	}
}

type harness struct {
	t   *testing.T
	svc *app.Services
}

func newHarness(t *testing.T) *harness {
	RequireRoot = false
	t.Cleanup(func() { RequireRoot = true })
	return &harness{t: t, svc: newServices(t)}
}

// run executes a command line against the harness's services, which are
// rebuilt per call like separate processes (so dry-run state never leaks).
func (h *harness) run(line ...string) (string, string, int) {
	var out, errOut bytes.Buffer
	build := func(string) (*app.Services, error) {
		svc := *h.svc
		w := *h.svc.Snippets
		hs := *h.svc.HostService
		svc.Snippets, svc.HostService = &w, &hs
		// Services that hold a writer or host service need the copies,
		// so a dry run records instead of writing.
		svc.IPLists = &iplist.Service{Store: h.svc.IPLists.Store, Manager: h.svc.Manager, Writer: &w, OtherUsers: h.svc.IPLists.OtherUsers}
		svc.RealIP = &realip.Service{Dir: h.svc.RealIP.Dir, Writer: &w}
		svc.Presets = &presets.Service{SnippetsDir: h.svc.Presets.SnippetsDir, HTTPDir: h.svc.Presets.HTTPDir, CacheDir: h.svc.Presets.CacheDir, Writer: &w}
		svc.Maintenance = &maintenance.Service{HTTPDir: h.svc.Maintenance.HTTPDir, Hosts: &hs, Writer: &w}
		svc.Upstreams = &upstreams.Service{Dir: h.svc.Upstreams.Dir, Manager: h.svc.Manager, Writer: &w}
		ss := *h.svc.Streams
		ss.Writer = &w
		svc.Streams = &ss
		gb := *h.svc.GeoBlock
		gb.Writer = &w
		svc.GeoBlock = &gb
		rl := *h.svc.RateLimits
		rl.Writer = &w
		svc.RateLimits = &rl
		return &svc, nil
	}
	code := Run(line, &out, &errOut, build, "unused")
	return out.String(), errOut.String(), code
}

func (h *harness) ok(line ...string) string {
	h.t.Helper()
	out, errOut, code := h.run(line...)
	if code != 0 {
		h.t.Fatalf("gonix %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(line, " "), code, out, errOut)
	}
	return out
}

func (h *harness) fails(code int, line ...string) string {
	h.t.Helper()
	out, errOut, got := h.run(line...)
	if got != code {
		h.t.Fatalf("gonix %s: exit %d, want %d\nstdout: %s\nstderr: %s", strings.Join(line, " "), got, code, out, errOut)
	}
	return out + errOut
}

func TestHostLifecycle(t *testing.T) {
	h := newHarness(t)
	sites := h.svc.Config.Nginx.SitesAvailable

	// Dry run: a diff, nothing written.
	out := h.ok("host", "add", "api.example.com", "--upstream", "http://127.0.0.1:3000", "--websocket", "--dry-run")
	if !strings.Contains(out, "+    server_name api.example.com;") || !strings.Contains(out, "enable host api.example.com") {
		t.Errorf("dry-run output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(sites, "api.example.com")); !os.IsNotExist(err) {
		t.Fatal("dry run created the host")
	}

	h.ok("host", "add", "api.example.com", "--upstream", "http://127.0.0.1:3000", "--websocket", "--alias", "www.api.example.com")
	var list []hostJSON
	if err := json.Unmarshal([]byte(h.ok("host", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Enabled || list[0].Locations[0].Proxy.Upstream != "http://127.0.0.1:3000" || !list[0].Locations[0].Proxy.WebSocket {
		t.Fatalf("host list = %+v", list)
	}

	// Settings by alias; presets and an IP list.
	h.ok("iplist", "create", "office", "--mode", "allow", "--entries", "10.0.0.0/8")
	h.ok("host", "set", "www.api.example.com", "--presets", "security-headers,gzip", "--ip-list", "office", "--upstream", "127.0.0.1:4000")
	var shown hostJSON
	json.Unmarshal([]byte(h.ok("host", "show", "api.example.com", "--json")), &shown)
	if shown.IPList != "office" || len(shown.Presets) != 2 || shown.Locations[0].Proxy.Upstream != "http://127.0.0.1:4000" {
		t.Errorf("after set: %+v", shown)
	}
	if _, err := os.Stat(filepath.Join(h.svc.Config.ErrorPages.SnippetsDirectory, "gonix-preset-gzip.conf")); err != nil {
		t.Error("preset snippet not written")
	}
	h.fails(1, "host", "set", "api.example.com", "--ip-list", "missing")
	h.fails(1, "host", "set", "api.example.com", "--presets", "hsts") // needs SSL

	// Paths, maintenance, delete.
	h.ok("host", "path", "add", "api.example.com", "/admin/", "--upstream", "http://127.0.0.1:9000", "--ip-list", "office")
	h.ok("host", "set", "api.example.com", "--maintenance", "on", "--bypass", "203.0.113.5")
	json.Unmarshal([]byte(h.ok("host", "show", "api.example.com", "--json")), &shown)
	if !shown.Maintenance || len(shown.Locations) != 2 || shown.Locations[1].IPList != "office" {
		t.Errorf("after path/maintenance: %+v", shown)
	}
	h.ok("host", "path", "remove", "api.example.com", "/admin/")
	h.ok("host", "disable", "api.example.com")

	out = h.fails(1, "host", "delete", "api.example.com")
	if !strings.Contains(out, "--yes") {
		t.Errorf("delete without --yes: %s", out)
	}
	h.ok("host", "delete", "api.example.com", "--yes")
	if _, err := os.Stat(filepath.Join(sites, "api.example.com")); !os.IsNotExist(err) {
		t.Error("host not deleted")
	}

	// The audit log attributes changes to the CLI.
	audit := h.ok("audit", "--json", "--lines", "50")
	if !strings.Contains(audit, `"actor": "cli"`) {
		t.Errorf("audit entries lack the cli actor:\n%s", audit)
	}
}

func TestRedirectStaticAndGroups(t *testing.T) {
	h := newHarness(t)
	h.ok("host", "add", "old.example.com", "--redirect", "example.com", "--code", "308", "--alias", "www.old.example.com")
	h.ok("host", "add", "static.example.com", "--root", "/srv/www")
	h.ok("upstream", "create", "app", "--servers", "10.0.0.1:80, 10.0.0.2:80 backup", "--method", "least_conn")
	h.ok("host", "add", "lb.example.com", "--group", "app")
	h.fails(1, "host", "add", "x.example.com", "--group", "missing")
	h.fails(2, "host", "add", "y.example.com")
	h.fails(2, "host", "add", "y.example.com", "--root", "/a", "--upstream", "http://b:1")

	var list []hostJSON
	json.Unmarshal([]byte(h.ok("host", "list", "--json")), &list)
	got := map[string]hostJSON{}
	for _, x := range list {
		got[x.Name] = x
	}
	if r := got["old.example.com"].Redirect; r == nil || r.Target != "https://example.com" || r.Code != 308 || !r.PreservePath {
		t.Errorf("redirect = %+v", r)
	}
	if got["static.example.com"].Root != "/srv/www" || got["lb.example.com"].Locations[0].Proxy.Group != "app" {
		t.Errorf("hosts = %+v", got)
	}
	out := h.fails(1, "upstream", "delete", "app", "--yes")
	if !strings.Contains(out, "still used by") {
		t.Errorf("deleting a used group: %s", out)
	}
}

func TestResourcesAndJSONErrors(t *testing.T) {
	h := newHarness(t)
	h.ok("ratelimit", "create", "api", "--preset", "api")
	h.ok("ratelimit", "create", "login", "--rate", "5", "--per", "m", "--burst", "5")
	if out := h.ok("ratelimit", "list"); !strings.Contains(out, "5 req/min, burst 5") {
		t.Errorf("ratelimit list:\n%s", out)
	}
	h.ok("iplist", "create", "blocked", "--mode", "block", "--entries", "203.0.113.7")
	h.ok("iplist", "add", "blocked", "198.51.100.1")
	h.ok("stream", "create", "pg", "--port", "5432", "--backend", "10.0.0.20:5432", "--ip-list", "blocked")
	h.fails(1, "iplist", "delete", "blocked", "--yes") // used by the stream
	h.ok("realip", "cloudflare")
	if out := h.ok("realip", "show", "--json"); !strings.Contains(out, `"cloudflare": true`) {
		t.Errorf("realip show:\n%s", out)
	}
	h.ok("preset", "list")

	// Errors are JSON too, with a non-zero exit code.
	out, _, code := h.run("host", "show", "nope.example.com", "--json")
	var e struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if code != 1 || json.Unmarshal([]byte(out), &e) != nil || e.OK || !strings.Contains(e.Error, "not found") {
		t.Errorf("JSON error = %q (exit %d)", out, code)
	}
	h.fails(2, "host", "list", "--bogus")
	h.fails(2, "host", "list", "--dry-run") // not a write command
	if out := h.fails(2, "host", "add", "--help"); !strings.Contains(out, "-upstream") {
		t.Errorf("--help output:\n%s", out)
	}
	h.fails(2, "nonsense")
}

func TestExportImport(t *testing.T) {
	src := newHarness(t)
	src.ok("iplist", "create", "office", "--mode", "allow", "--entries", "10.0.0.0/8")
	src.ok("host", "add", "example.com", "--upstream", "http://127.0.0.1:3000", "--ip-list", "office")
	archive := filepath.Join(t.TempDir(), "export.tar.gz")
	src.ok("export", archive)

	dst := newHarness(t)
	out := dst.ok("import", archive, "--dry-run")
	if !strings.Contains(out, "gonix-ip-office.conf") || !strings.Contains(out, "enable host example.com") {
		t.Errorf("import dry run:\n%s", out)
	}
	dst.ok("import", archive)
	var shown hostJSON
	json.Unmarshal([]byte(dst.ok("host", "show", "example.com", "--json")), &shown)
	if !shown.Enabled || shown.IPList != "office" {
		t.Errorf("imported host = %+v", shown)
	}
	site, _ := os.ReadFile(filepath.Join(dst.svc.Config.Nginx.SitesAvailable, "example.com"))
	if !strings.Contains(string(site), dst.svc.Config.ErrorPages.SnippetsDirectory) {
		t.Errorf("paths not rewritten for the new server:\n%s", site)
	}
}

func TestCheckAndSkill(t *testing.T) {
	h := newHarness(t)
	h.ok("host", "add", "a.example.com", "--upstream", "http://127.0.0.1:1")
	h.ok("host", "add", "b.example.com", "--upstream", "http://127.0.0.1:1", "--alias", "a.example.com")
	out := h.fails(1, "check", "--no-upstreams")
	if !strings.Contains(out, "used by 2 enabled hosts") {
		t.Errorf("check:\n%s", out)
	}
	var findings []map[string]string
	if err := json.Unmarshal([]byte(h.ok("check", "--no-upstreams", "--json")), &findings); err != nil || len(findings) == 0 {
		t.Errorf("check --json = %v, %v", findings, err)
	}

	dir := t.TempDir()
	h.ok("skill", "install", "--dir", dir)
	data, err := os.ReadFile(filepath.Join(dir, "gonix", "SKILL.md"))
	if err != nil || !strings.HasPrefix(string(data), "---\nname: gonix\n") {
		t.Errorf("installed skill = %.40q, %v", data, err)
	}
}

// Every command mentioned in the skill must exist, so the instructions an
// agent follows never drift from the CLI.
func TestSkillOnlyMentionsRealCommands(t *testing.T) {
	data, err := os.ReadFile("../../skills/gonix/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	type mention struct{ group, text string } // group: first command word of the line
	var mentions []mention
	add := func(parts []string) {
		group := strings.Fields(strings.TrimPrefix(strings.TrimSpace(parts[0]), "gonix "))[0]
		for _, p := range parts {
			mentions = append(mentions, mention{group, p})
		}
	}
	inCode := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			// A line may list several commands: "a | b".
			if i := strings.Index(line, "gonix "); i >= 0 && !strings.HasPrefix(strings.TrimSpace(line), "#") {
				add(strings.Split(line[i:], " | "))
			}
			continue
		}
		for _, span := range strings.Split(line, "`")[1:] {
			if strings.HasPrefix(span, "gonix ") {
				add([]string{span})
			}
		}
	}
	checked := 0
	for _, m := range mentions {
		words := strings.Fields(strings.TrimPrefix(strings.TrimSpace(m.text), "gonix "))
		var cmd []string
		for _, w := range words {
			if strings.ContainsAny(w[:1], "<-\"[#(") {
				break
			}
			cmd = append(cmd, w)
		}
		if len(cmd) == 0 || cmd[0] == "help" {
			continue
		}
		// "enable|disable" lists alternatives for the last word.
		last := cmd[len(cmd)-1]
		for _, alt := range strings.Split(last, "|") {
			c := append(append([]string{}, cmd[:len(cmd)-1]...), alt)
			if _, _, ok := find(c); !ok {
				// Continuation fragments such as "delete <name>" belong to
				// the command group mentioned first on the line.
				if _, _, ok := find(append([]string{m.group}, c...)); !ok {
					t.Errorf("SKILL.md mentions unknown command %q (%s)", strings.Join(c, " "), strings.TrimSpace(m.text))
				}
			}
			checked++
		}
	}
	if checked < 30 {
		t.Errorf("only %d command mentions checked; the parser is probably broken", checked)
	}
}
