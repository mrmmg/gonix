// Package accesslist implements reusable HTTP Basic Authentication
// definitions ("Access Lists"), each backed by an htpasswd-format file that
// Nginx reads directly via auth_basic_user_file.
package accesslist

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

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
	// Group owns the directory and every htpasswd file. It must be the
	// Nginx worker group, since workers (not the root master process) open
	// auth_basic_user_file on each request. Empty leaves ownership as is.
	Group string
	gid   int // -1 when Group is empty
}

// NewStore returns a Store rooted at dir, creating it if necessary. When
// group is non-empty, the directory, its existing htpasswd files and its
// parent directories are made readable (or traversable) by that group.
func NewStore(dir, group string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating access list directory: %w", err)
	}
	s := &Store{Directory: dir, Group: group, gid: -1}
	if group == "" {
		return s, nil
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return nil, fmt.Errorf("looking up access list group %q: %w", group, err)
	}
	if s.gid, err = strconv.Atoi(g.Gid); err != nil {
		return nil, fmt.Errorf("parsing gid of group %q: %w", group, err)
	}
	if err := s.fixPermissions(); err != nil {
		return nil, fmt.Errorf("setting access list permissions for group %q: %w", group, err)
	}
	return s, nil
}

// fixPermissions gives s.gid read access to the directory and every
// htpasswd file in it. The directory gets the setgid bit, so files created
// later inherit the group too.
func (s *Store) fixPermissions() error {
	if err := ensureTraversable(filepath.Dir(s.Directory), s.gid); err != nil {
		return err
	}
	if err := os.Chown(s.Directory, -1, s.gid); err != nil {
		return err
	}
	if err := os.Chmod(s.Directory, 0o750|os.ModeSetgid); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".htpasswd") {
			continue
		}
		if err := s.secure(filepath.Join(s.Directory, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// ensureTraversable makes sure gid can traverse dir and every ancestor,
// adding only the "others execute" bit (never read) where neither group
// nor others could traverse it, e.g. /etc/gonix with mode 0750 root:root.
func ensureTraversable(dir string, gid int) error {
	for {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		perm := info.Mode().Perm()
		groupOK := false
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			groupOK = int(st.Gid) == gid && perm&0o010 != 0
		}
		if perm&0o001 == 0 && !groupOK {
			special := info.Mode() & (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
			if err := os.Chmod(dir, special|perm|0o001); err != nil {
				return err
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

// secure gives an htpasswd file mode 0640 owned by the store's group.
func (s *Store) secure(path string) error {
	if s.gid < 0 {
		return nil
	}
	if err := os.Chown(path, -1, s.gid); err != nil {
		return err
	}
	return os.Chmod(path, 0o640)
}

// writeFile writes an htpasswd file and applies the store's ownership.
func (s *Store) writeFile(name string, data []byte) error {
	path := s.path(name)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return err
	}
	if err := s.secure(path); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", path, err)
	}
	return nil
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
	return s.writeFile(name, nil)
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
	return s.writeFile(name, []byte(b.String()))
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
