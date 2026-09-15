// Package backup implements snapshot and rollback of Nginx host
// configuration files, used by internal/hosts to make changes safe: a
// snapshot is taken before any write, and restored automatically if the new
// configuration fails `nginx -t`.
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Backuper stores and restores point-in-time snapshots of individual
// configuration files under Directory.
type Backuper struct {
	Directory string
	KeepCount int // how many snapshots to retain per file; 0 means unlimited
}

// New returns a Backuper writing snapshots under dir.
func New(dir string, keepCount int) *Backuper {
	return &Backuper{Directory: dir, KeepCount: keepCount}
}

// Snapshot is a single stored backup of a file.
type Snapshot struct {
	Path      string // full path to the stored snapshot file
	FileName  string // original file name that was backed up
	Timestamp time.Time
}

// Save stores a copy of the file at sourcePath (identified by fileName, e.g.
// the host's config file name) into the backup directory, timestamped so
// multiple generations can coexist. If sourcePath does not exist yet (e.g.
// creating a brand new host), Save is a no-op and returns an empty Snapshot.
func (b *Backuper) Save(fileName, sourcePath string) (Snapshot, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, nil
		}
		return Snapshot{}, fmt.Errorf("reading %s for backup: %w", sourcePath, err)
	}

	if err := os.MkdirAll(b.Directory, 0o755); err != nil {
		return Snapshot{}, fmt.Errorf("creating backup directory: %w", err)
	}

	ts := time.Now()
	backupName := fmt.Sprintf("%s.%s.bak", fileName, ts.Format("20060102-150405"))
	backupPath := filepath.Join(b.Directory, backupName)
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return Snapshot{}, fmt.Errorf("writing backup %s: %w", backupPath, err)
	}

	if b.KeepCount > 0 {
		if err := b.prune(fileName); err != nil {
			return Snapshot{}, err
		}
	}

	return Snapshot{Path: backupPath, FileName: fileName, Timestamp: ts}, nil
}

// Restore writes a snapshot's content back to destPath, overwriting whatever
// is currently there.
func (b *Backuper) Restore(snap Snapshot, destPath string) error {
	data, err := os.ReadFile(snap.Path)
	if err != nil {
		return fmt.Errorf("reading backup %s: %w", snap.Path, err)
	}
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return fmt.Errorf("restoring backup to %s: %w", destPath, err)
	}
	return nil
}

// List returns all stored snapshots for fileName, newest first.
func (b *Backuper) List(fileName string) ([]Snapshot, error) {
	entries, err := os.ReadDir(b.Directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading backup directory: %w", err)
	}

	prefix := fileName + "."
	var snaps []Snapshot
	for _, e := range entries {
		if e.IsDir() || len(e.Name()) <= len(prefix) || e.Name()[:len(prefix)] != prefix {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		snaps = append(snaps, Snapshot{
			Path:      filepath.Join(b.Directory, e.Name()),
			FileName:  fileName,
			Timestamp: info.ModTime(),
		})
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Timestamp.After(snaps[j].Timestamp) })
	return snaps, nil
}

// prune removes old snapshots of fileName beyond KeepCount, oldest first.
func (b *Backuper) prune(fileName string) error {
	snaps, err := b.List(fileName)
	if err != nil {
		return err
	}
	if len(snaps) <= b.KeepCount {
		return nil
	}
	for _, s := range snaps[b.KeepCount:] {
		if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("pruning old backup %s: %w", s.Path, err)
		}
	}
	return nil
}
