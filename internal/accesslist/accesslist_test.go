package accesslist

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestCreateAddRemoveUser(t *testing.T) {
	store, err := NewStore(t.TempDir())
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
	store, _ := NewStore(t.TempDir())
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
	store, _ := NewStore(t.TempDir())
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
	store, _ := NewStore(t.TempDir())
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
