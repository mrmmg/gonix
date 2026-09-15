package hosts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shiva/gonix/internal/audit"
	"github.com/shiva/gonix/internal/backup"
	"github.com/shiva/gonix/internal/nginx"
)

// fakeTester lets tests control the outcome of `nginx -t` without a real
// Nginx installation.
type fakeTester struct {
	ok     bool
	output string
}

func (f fakeTester) Test(ctx context.Context) (nginx.Result, error) {
	return nginx.Result{OK: f.ok, Output: f.output}, nil
}

type fakeReloader struct {
	called bool
	err    error
}

func (f *fakeReloader) Reload(ctx context.Context) error {
	f.called = true
	return f.err
}

func newTestService(t *testing.T, ok bool) (*Service, string, string) {
	t.Helper()
	available := t.TempDir()
	enabled := t.TempDir()
	backupDir := t.TempDir()

	renderer, err := nginx.NewRenderer(t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	mgr := nginx.NewManager(available, enabled, renderer)
	auditLogger, err := audit.NewLogger(filepath.Join(t.TempDir(), "audit.log"))
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	svc := &Service{
		Manager:  mgr,
		Tester:   fakeTester{ok: ok},
		Reloader: &fakeReloader{},
		Backups:  backup.New(backupDir, 0),
		Audit:    auditLogger,
	}
	return svc, available, enabled
}

func testHost(name string) nginx.Host {
	return nginx.Host{
		ServerName: name,
		Mode:       nginx.ModeReverseProxy,
		Listen:     80,
		AccessLog:  true,
		ErrorLog:   true,
		Locations: []nginx.Location{
			{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}},
		},
	}
}

func TestCreateHostSuccess(t *testing.T) {
	svc, available, _ := newTestService(t, true)
	_, err := svc.CreateHost(context.Background(), testHost("example.com"))
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(available, "example.com")); err != nil {
		t.Fatalf("expected config file to be written: %v", err)
	}
}

func TestCreateHostRejectsInvalidDomain(t *testing.T) {
	svc, _, _ := newTestService(t, true)
	h := testHost("not a domain")
	if _, err := svc.CreateHost(context.Background(), h); err == nil {
		t.Fatal("expected validation error for invalid domain")
	}
}

func TestCreateHostRollsBackOnValidationFailure(t *testing.T) {
	svc, available, _ := newTestService(t, true)

	// First, successfully create a host.
	if _, err := svc.CreateHost(context.Background(), testHost("example.com")); err != nil {
		t.Fatalf("initial CreateHost: %v", err)
	}
	originalData, err := os.ReadFile(filepath.Join(available, "example.com"))
	if err != nil {
		t.Fatalf("reading original config: %v", err)
	}

	// Now attempt an update that will fail `nginx -t`.
	svc.Tester = fakeTester{ok: false, output: "nginx: [emerg] test failure"}
	h := testHost("example.com")
	h.Locations[0].Proxy.UpstreamPort = 9090

	result, err := svc.UpdateHost(context.Background(), h)
	if err == nil {
		t.Fatal("expected error from failed validation")
	}
	if !result.RolledBack {
		t.Fatal("expected configuration to be rolled back")
	}

	restored, err := os.ReadFile(filepath.Join(available, "example.com"))
	if err != nil {
		t.Fatalf("reading restored config: %v", err)
	}
	if string(restored) != string(originalData) {
		t.Fatal("expected restored config to match original after rollback")
	}
}

func TestEnableDisableHost(t *testing.T) {
	svc, _, enabled := newTestService(t, true)
	if _, err := svc.CreateHost(context.Background(), testHost("example.com")); err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	if err := svc.EnableHost(context.Background(), "example.com"); err != nil {
		t.Fatalf("EnableHost: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(enabled, "example.com")); err != nil {
		t.Fatalf("expected symlink to be created: %v", err)
	}

	if err := svc.DisableHost(context.Background(), "example.com"); err != nil {
		t.Fatalf("DisableHost: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(enabled, "example.com")); !os.IsNotExist(err) {
		t.Fatalf("expected symlink to be removed, got err=%v", err)
	}
}

func TestDeleteHostNeverDeletesOnDisableAlone(t *testing.T) {
	svc, available, enabled := newTestService(t, true)
	if _, err := svc.CreateHost(context.Background(), testHost("example.com")); err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if err := svc.EnableHost(context.Background(), "example.com"); err != nil {
		t.Fatalf("EnableHost: %v", err)
	}
	if err := svc.DisableHost(context.Background(), "example.com"); err != nil {
		t.Fatalf("DisableHost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(available, "example.com")); err != nil {
		t.Fatalf("config file must survive disable: %v", err)
	}
	_ = enabled
}
