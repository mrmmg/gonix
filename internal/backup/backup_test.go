package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndRestore(t *testing.T) {
	srcDir := t.TempDir()
	backupDir := t.TempDir()
	src := filepath.Join(srcDir, "example.com")

	if err := os.WriteFile(src, []byte("version 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := New(backupDir, 0)
	snap, err := b.Save("example.com", src)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if snap.Path == "" {
		t.Fatal("expected non-empty snapshot path")
	}

	if err := os.WriteFile(src, []byte("version 2 (broken)"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := b.Restore(snap, src); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "version 1" {
		t.Errorf("expected restored content 'version 1', got %q", data)
	}
}

func TestSaveNoOpWhenSourceMissing(t *testing.T) {
	b := New(t.TempDir(), 0)
	snap, err := b.Save("missing.com", filepath.Join(t.TempDir(), "missing.com"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if snap.Path != "" {
		t.Errorf("expected empty snapshot for missing source, got %+v", snap)
	}
}

func TestListReturnsNewestFirst(t *testing.T) {
	srcDir := t.TempDir()
	backupDir := t.TempDir()
	src := filepath.Join(srcDir, "example.com")
	b := New(backupDir, 0)

	if err := os.WriteFile(src, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Save("example.com", src); err != nil {
		t.Fatalf("Save: %v", err)
	}
	time.Sleep(1100 * time.Millisecond) // ensure a distinct timestamp in the file name

	if err := os.WriteFile(src, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Save("example.com", src); err != nil {
		t.Fatalf("Save: %v", err)
	}

	snaps, err := b.List("example.com")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}
	if !snaps[0].Timestamp.After(snaps[1].Timestamp) && !snaps[0].Timestamp.Equal(snaps[1].Timestamp) {
		t.Errorf("expected newest-first ordering, got %v then %v", snaps[0].Timestamp, snaps[1].Timestamp)
	}
}

func TestPruneKeepsOnlyKeepCount(t *testing.T) {
	srcDir := t.TempDir()
	backupDir := t.TempDir()
	src := filepath.Join(srcDir, "example.com")
	b := New(backupDir, 2)

	for i := 0; i < 4; i++ {
		if err := os.WriteFile(src, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := b.Save("example.com", src); err != nil {
			t.Fatalf("Save: %v", err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	snaps, err := b.List("example.com")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected pruning to keep 2 snapshots, got %d", len(snaps))
	}
}
