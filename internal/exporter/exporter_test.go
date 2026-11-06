package exporter

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/snippet"
)

type okTester struct{ ok bool }

func (t okTester) Test(context.Context) (nginx.Result, error) {
	return nginx.Result{OK: t.ok, Output: "nginx says"}, nil
}

type fixture struct {
	svc   *Service
	calls *[]string
	cfg   *config.Config
}

func newFixture(t *testing.T, instance string, nginxOK bool) fixture {
	t.Helper()
	cfg := config.Default()
	cfg.Instance = instance
	cfg.Monitoring.Listen = freeAddr(t)
	cfg.Nginx.HTTPSnippetsDir = t.TempDir()
	var calls []string
	svc := New(cfg, filepath.Join(t.TempDir(), "gonix.yaml"), &snippet.Writer{Tester: okTester{nginxOK}})
	svc.Binary = "/usr/local/bin/gonix"
	svc.UnitDir = t.TempDir()
	svc.Systemctl = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "is-active" {
			return "inactive", nil
		}
		return "", nil
	}
	return fixture{svc, &calls, cfg}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func TestUnitNamesAndContentPerInstance(t *testing.T) {
	def := newFixture(t, "", true).svc
	if def.UnitName() != "gonix-exporter" {
		t.Errorf("unit = %s", def.UnitName())
	}
	st := newFixture(t, "staging", true).svc
	if st.UnitName() != "gonix-exporter-staging" {
		t.Errorf("unit = %s", st.UnitName())
	}
	u := st.Unit()
	if !strings.Contains(u, "ExecStart=/usr/local/bin/gonix exporter run --config "+st.ConfigPath) {
		t.Errorf("unit:\n%s", u)
	}
	if strings.Contains(u, st.Config.Monitoring.Listen) {
		t.Error("settings must not be in the unit; changing them would mean editing it")
	}
}

func TestEnableWritesUnitAndStarts(t *testing.T) {
	f := newFixture(t, "", true)
	if err := f.svc.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.svc.UnitDir, "gonix-exporter.service")); err != nil {
		t.Fatal("unit not written")
	}
	got := strings.Join(*f.calls, "|")
	if !strings.Contains(got, "daemon-reload") || !strings.Contains(got, "enable --now gonix-exporter") {
		t.Errorf("systemctl calls: %s", got)
	}
}

func TestEnableRefusesABusyPort(t *testing.T) {
	f := newFixture(t, "", true)
	ln, err := net.Listen("tcp", f.cfg.Monitoring.Listen)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := f.svc.Enable(context.Background()); err == nil || !strings.Contains(err.Error(), "exporter set --listen") {
		t.Errorf("err = %v", err)
	}
}

func TestApplySavesRestartsAndRewritesNothingInTheUnit(t *testing.T) {
	f := newFixture(t, "", true)
	f.svc.Systemctl = func(_ context.Context, args ...string) (string, error) {
		*f.calls = append(*f.calls, strings.Join(args, " "))
		if args[0] == "is-active" {
			return "active", nil
		}
		return "", nil
	}
	f.svc.ensureUnit(context.Background())
	unitBefore := f.svc.Unit()

	m := f.cfg.Monitoring
	m.Listen = freeAddr(t)
	m.Hosts = []string{"*.example.com"}
	if err := f.svc.Apply(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(f.svc.ConfigPath)
	if err != nil || loaded.Monitoring.Listen != m.Listen || loaded.Monitoring.Hosts[0] != "*.example.com" {
		t.Fatalf("not saved: %v %+v", err, loaded)
	}
	if f.svc.Unit() != unitBefore {
		t.Error("changing the port must not change the unit")
	}
	if !strings.Contains(strings.Join(*f.calls, "|"), "restart gonix-exporter") {
		t.Errorf("running exporter not restarted: %v", *f.calls)
	}
}

func TestApplyRejectsInvalidSettings(t *testing.T) {
	f := newFixture(t, "", true)
	m := f.cfg.Monitoring
	m.Collectors = []string{"bogus"}
	if err := f.svc.Apply(context.Background(), m); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(f.svc.ConfigPath); err == nil {
		t.Error("invalid settings were saved")
	}
}

func TestStubStatusIsWrittenAndRolledBack(t *testing.T) {
	f := newFixture(t, "", true)
	m := f.cfg.Monitoring
	m.StubStatus.Enabled = true
	if err := f.svc.Apply(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(f.svc.StubStatusPath())
	if err != nil || !strings.Contains(string(data), "stub_status;") || !strings.Contains(string(data), "deny all;") {
		t.Fatalf("snippet: %v\n%s", err, data)
	}
	if f.svc.StubStatusURL() != "http://"+m.StubStatus.Listen+"/stub_status" {
		t.Errorf("url = %s", f.svc.StubStatusURL())
	}
	m.StubStatus.Enabled = false
	if err := f.svc.Apply(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.svc.StubStatusPath()); err == nil {
		t.Error("snippet not removed")
	}

	// Nginx without the module refuses the block: nothing is kept or saved.
	bad := newFixture(t, "", false)
	m = bad.cfg.Monitoring
	m.StubStatus.Enabled = true
	if err := bad.svc.Apply(context.Background(), m); err == nil {
		t.Fatal("expected nginx -t to fail")
	}
	if _, err := os.Stat(bad.svc.StubStatusPath()); err == nil {
		t.Error("snippet kept although nginx -t failed")
	}
	if bad.cfg.Monitoring.StubStatus.Enabled {
		t.Error("settings changed although nginx -t failed")
	}
}

func TestUninstallRemovesUnitButNotSettings(t *testing.T) {
	f := newFixture(t, "", true)
	f.svc.Enable(context.Background())
	if err := f.svc.Uninstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.svc.UnitDir, "gonix-exporter.service")); err == nil {
		t.Error("unit still there")
	}
}
