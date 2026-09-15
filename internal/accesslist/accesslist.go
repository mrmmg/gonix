// Package accesslist implements reusable HTTP Basic Authentication
// definitions ("Access Lists"), each backed by an htpasswd-format file that
// Nginx reads directly via auth_basic_user_file.
package accesslist

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// AccessList is a named collection of Basic Auth users.
type AccessList struct {
	Name  string
	Users []string // usernames only; password hashes live in the htpasswd file
}

// Store manages access list definitions and their htpasswd files under a
// single directory. Each access list "name" maps to a file
// "<directory>/<name>.htpasswd" in standard htpasswd (bcrypt) format.
type Store struct {
	Directory string
}

// NewStore returns a Store rooted at dir, creating it if necessary.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating access list directory: %w", err)
	}
	return &Store{Directory: dir}, nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.Directory, name+".htpasswd")
}

// List returns every defined access list, sorted by name.
func (s *Store) List() ([]AccessList, error) {
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading access list directory: %w", err)
	}
	var lists []AccessList
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".htpasswd") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".htpasswd")
		al, err := s.Get(name)
		if err != nil {
			continue
		}
		lists = append(lists, al)
	}
	sort.Slice(lists, func(i, j int) bool { return lists[i].Name < lists[j].Name })
	return lists, nil
}

// Get loads a single access list's user names (without password hashes).
func (s *Store) Get(name string) (AccessList, error) {
	entries, err := s.readEntries(name)
	if err != nil {
		return AccessList{}, err
	}
	al := AccessList{Name: name}
	for _, e := range entries {
		al.Users = append(al.Users, e.user)
	}
	return al, nil
}

// Exists reports whether an access list with the given name has been created.
func (s *Store) Exists(name string) bool {
	_, err := os.Stat(s.path(name))
	return err == nil
}

// Create makes a new, empty access list. It fails if one already exists with
// that name.
func (s *Store) Create(name string) error {
	if s.Exists(name) {
		return fmt.Errorf("access list %q already exists", name)
	}
	return os.WriteFile(s.path(name), nil, 0o640)
}

// Delete removes an access list and its htpasswd file entirely.
func (s *Store) Delete(name string) error {
	if err := os.Remove(s.path(name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting access list %q: %w", name, err)
	}
	return nil
}

type htEntry struct {
	user string
	hash string
}

func (s *Store) readEntries(name string) ([]htEntry, error) {
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("access list %q does not exist", name)
		}
		return nil, fmt.Errorf("reading access list %q: %w", name, err)
	}
	var entries []htEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		entries = append(entries, htEntry{user: parts[0], hash: parts[1]})
	}
	return entries, nil
}

func (s *Store) writeEntries(name string, entries []htEntry) error {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s:%s\n", e.user, e.hash)
	}
	return os.WriteFile(s.path(name), []byte(b.String()), 0o640)
}

// AddUser adds a new user with the given plaintext password to the access
// list, hashing it with bcrypt (an htpasswd-compatible algorithm supported
// by Nginx). It fails if the user already exists; use SetPassword to change
// an existing user's password.
func (s *Store) AddUser(listName, username, password string) error {
	entries, err := s.readEntries(listName)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.user == username {
			return fmt.Errorf("user %q already exists in access list %q", username, listName)
		}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	entries = append(entries, htEntry{user: username, hash: hash})
	return s.writeEntries(listName, entries)
}

// RemoveUser deletes a user from the access list.
func (s *Store) RemoveUser(listName, username string) error {
	entries, err := s.readEntries(listName)
	if err != nil {
		return err
	}
	out := entries[:0]
	found := false
	for _, e := range entries {
		if e.user == username {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return fmt.Errorf("user %q not found in access list %q", username, listName)
	}
	return s.writeEntries(listName, out)
}

// SetPassword updates an existing user's password.
func (s *Store) SetPassword(listName, username, password string) error {
	entries, err := s.readEntries(listName)
	if err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	found := false
	for i, e := range entries {
		if e.user == username {
			entries[i].hash = hash
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("user %q not found in access list %q", username, listName)
	}
	return s.writeEntries(listName, entries)
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// GenerateRandomPassword returns a URL-safe random password suitable as a
// default when the operator wants one generated rather than typed.
func GenerateRandomPassword() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating random password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
