package accesslist

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestCreateAddRemoveUser(t *testing.T) {
	store, err := NewStore(t.TempDir(), "")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if err := store.Create("admin-panel"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Create("admin-panel"); err == nil {
		t.Fatal("expected error creating duplicate access list")
	}

	if err := store.AddUser("admin-panel", "admin", "s3cret-password"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if err := store.AddUser("admin-panel", "admin", "other"); err == nil {
		t.Fatal("expected error adding duplicate user")
	}

	al, err := store.Get("admin-panel")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(al.Users) != 1 || al.Users[0] != "admin" {
		t.Fatalf("expected one user 'admin', got %v", al.Users)
	}

	if err := store.RemoveUser("admin-panel", "admin"); err != nil {
		t.Fatalf("RemoveUser: %v", err)
	}
	al, _ = store.Get("admin-panel")
	if len(al.Users) != 0 {
		t.Fatalf("expected no users after removal, got %v", al.Users)
	}
}

func TestPasswordsAreHashedNotPlaintext(t *testing.T) {
	store, _ := NewStore(t.TempDir(), "")
	_ = store.Create("list1")
	_ = store.AddUser("list1", "operator", "hunter2")

	entries, err := store.readEntries("list1")
	if err != nil {
		t.Fatalf("readEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if strings.Contains(entries[0].hash, "hunter2") {
		t.Fatal("password hash must not contain the plaintext password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(entries[0].hash), []byte("hunter2")); err != nil {
		t.Fatalf("stored hash does not match original password: %v", err)
	}
}

func TestSetPassword(t *testing.T) {
	store, _ := NewStore(t.TempDir(), "")
	_ = store.Create("list1")
	_ = store.AddUser("list1", "operator", "first-password")

	if err := store.SetPassword("list1", "operator", "second-password"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	entries, _ := store.readEntries("list1")
	if err := bcrypt.CompareHashAndPassword([]byte(entries[0].hash), []byte("second-password")); err != nil {
		t.Fatal("password was not updated")
	}

	if err := store.SetPassword("list1", "nobody", "x"); err == nil {
		t.Fatal("expected error setting password for unknown user")
	}
}

func TestDeleteAccessList(t *testing.T) {
	store, _ := NewStore(t.TempDir(), "")
	_ = store.Create("list1")
	if !store.Exists("list1") {
		t.Fatal("expected list1 to exist")
	}
	if err := store.Delete("list1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Exists("list1") {
		t.Fatal("expected list1 to no longer exist")
	}
}

func TestStoreGivesNginxGroupReadAccess(t *testing.T) {
	// Use the current user's primary group: unprivileged processes may only
	// chgrp to groups they belong to.
	u, err := user.Current()
	if err != nil {
		t.Skipf("current user: %v", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Skipf("primary group: %v", err)
	}
	gid, _ := strconv.Atoi(g.Gid)

	parent := filepath.Join(t.TempDir(), "gonix")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "accesslists")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A file written before the group was configured must be repaired.
	if err := os.WriteFile(filepath.Join(dir, "old.htpasswd"), []byte("u:x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dir, g.Name)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Create("new"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.AddUser("new", "alice", "secret"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	if info, _ := os.Stat(parent); info.Mode().Perm()&0o001 == 0 {
		t.Errorf("parent %s not traversable: %v", parent, info.Mode())
	}
	info, _ := os.Stat(dir)
	if info.Mode()&os.ModeSetgid == 0 || info.Mode().Perm() != 0o750 {
		t.Errorf("directory mode = %v, want setgid 0750", info.Mode())
	}
	for _, name := range []string{"old.htpasswd", "new.htpasswd"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Errorf("%s mode = %v, want 0640", name, info.Mode().Perm())
		}
		if st := info.Sys().(*syscall.Stat_t); int(st.Gid) != gid {
			t.Errorf("%s gid = %d, want %d", name, st.Gid, gid)
		}
	}
}

func TestNewStoreUnknownGroup(t *testing.T) {
	if _, err := NewStore(t.TempDir(), "gonix-no-such-group-xyz"); err == nil {
		t.Fatal("expected an error for an unknown group")
	}
}
