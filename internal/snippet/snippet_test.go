package snippet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
)

type fakeTester struct{ ok bool }

func (f fakeTester) Test(context.Context) (nginx.Result, error) {
	return nginx.Result{OK: f.ok, Output: "nginx: [emerg] test"}, nil
}

type countingReloader struct{ n int }

func (r *countingReloader) Reload(context.Context) error { r.n++; return nil }

func TestWriteAndRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.conf")
	rl := &countingReloader{}
	w := &Writer{Tester: fakeTester{ok: true}, Reloader: rl}

	if err := w.Write(context.Background(), "test", "a", path, "allow all;\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "allow all;\n" {
		t.Errorf("content = %q", data)
	}
	if err := w.Remove(context.Background(), "test", "a", path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed, err=%v", err)
	}
	if err := w.Remove(context.Background(), "test", "a", path); err != nil {
		t.Errorf("removing a missing file should succeed: %v", err)
	}
	if rl.n != 3 {
		t.Errorf("reloads = %d, want 3", rl.n)
	}
}

func TestApplyRestoresEveryFileOnFailedTest(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.conf")
	created := filepath.Join(dir, "created.conf")
	removed := filepath.Join(dir, "removed.conf")
	for path, content := range map[string]string{existing: "old\n", removed: "keep me\n"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rl := &countingReloader{}
	w := &Writer{Tester: fakeTester{ok: false}, Reloader: rl}
	newContent := "new\n"
	err := w.Apply(context.Background(), "test", "x",
		File{Path: existing, Content: &newContent},
		File{Path: created, Content: &newContent},
		File{Path: removed},
	)
	if err == nil {
		t.Fatal("expected an error from the failed nginx -t")
	}
	if data, _ := os.ReadFile(existing); string(data) != "old\n" {
		t.Errorf("existing file not restored: %q", data)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("newly created file should be removed on rollback, err=%v", err)
	}
	if data, _ := os.ReadFile(removed); string(data) != "keep me\n" {
		t.Errorf("removed file not restored: %q", data)
	}
	if rl.n != 0 {
		t.Errorf("nginx must not be reloaded after a failed test, reloads = %d", rl.n)
	}
}
