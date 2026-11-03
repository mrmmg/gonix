package history

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
)

type fakeTester struct{ ok bool }

func (f fakeTester) Test(context.Context) (nginx.Result, error) {
	return nginx.Result{OK: f.ok, Output: "nginx: [emerg] broken"}, nil
}

func newHistory(t *testing.T) (*History, string) {
	t.Helper()
	if !Available() {
		t.Skip("git not installed")
	}
	work := t.TempDir()
	for _, d := range []string{"sites-available", "sites-enabled", "conf.d"} {
		os.MkdirAll(filepath.Join(work, d), 0o755)
	}
	return &History{GitDir: filepath.Join(t.TempDir(), "history.git"), WorkTree: work,
		Paths: []string{"sites-available", "sites-enabled", "conf.d", "gonix-streams"}}, work
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCommitLogShowRestore(t *testing.T) {
	h, work := newHistory(t)
	ctx := context.Background()
	site := filepath.Join(work, "sites-available", "example.com")

	write(t, site, "v1\n")
	os.Symlink(site, filepath.Join(work, "sites-enabled", "example.com"))
	if err := h.Commit(ctx, "host_created example.com"); err != nil {
		t.Fatal(err)
	}
	if err := h.Commit(ctx, "nothing changed"); err != nil {
		t.Fatal(err)
	}
	write(t, site, "v2\n")
	write(t, filepath.Join(work, "conf.d", "gonix-new.conf"), "new\n")
	if err := h.Commit(ctx, "host_updated example.com"); err != nil {
		t.Fatal(err)
	}

	log, err := h.Log(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, c := range log {
		msgs = append(msgs, c.Message)
	}
	if !reflect.DeepEqual(msgs, []string{"host_updated example.com", "host_created example.com"}) {
		t.Fatalf("log = %v", msgs)
	}
	diff, err := h.Show(ctx, log[0].Hash)
	if err != nil || !strings.Contains(diff, "-v1") || !strings.Contains(diff, "+v2") {
		t.Errorf("Show = %q, %v", diff, err)
	}

	// A restore that nginx rejects changes nothing.
	if err := h.Restore(ctx, log[1].Hash, fakeTester{ok: false}); err == nil {
		t.Fatal("expected the failed nginx -t to abort the restore")
	}
	if data, _ := os.ReadFile(site); string(data) != "v2\n" {
		t.Errorf("site after aborted restore = %q", data)
	}

	if err := h.Restore(ctx, log[1].Hash, fakeTester{ok: true}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(site); string(data) != "v1\n" {
		t.Errorf("site after restore = %q", data)
	}
	if _, err := os.Stat(filepath.Join(work, "conf.d", "gonix-new.conf")); !os.IsNotExist(err) {
		t.Error("a file added after the restored commit should be removed")
	}
	if target, err := os.Readlink(filepath.Join(work, "sites-enabled", "example.com")); err != nil || target != site {
		t.Errorf("symlink = %q, %v", target, err)
	}
	log, _ = h.Log(ctx, 1)
	if !strings.HasPrefix(log[0].Message, "Restored ") {
		t.Errorf("latest commit = %q", log[0].Message)
	}

	if err := h.Restore(ctx, "--evil", nil); err == nil {
		t.Error("expected an invalid hash to be rejected")
	}
}

func TestLogBeforeFirstCommit(t *testing.T) {
	h, _ := newHistory(t)
	if log, err := h.Log(context.Background(), 5); err != nil || len(log) != 0 {
		t.Errorf("Log = %v, %v", log, err)
	}
}

func TestTrackedPaths(t *testing.T) {
	got := TrackedPaths("/etc/nginx", "/etc/nginx/sites-available", "/etc/nginx/conf.d", "/etc/gonix/accesslists", "/etc/nginx", "/etc/nginx/conf.d")
	if !reflect.DeepEqual(got, []string{"sites-available", "conf.d"}) {
		t.Errorf("TrackedPaths = %v", got)
	}
}
