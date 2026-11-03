// Package history keeps a Git history of the Nginx configuration: every
// successful change made through GoNix becomes a commit, any commit can be
// inspected as a diff, and the configuration can be rolled back to it.
//
// The repository lives outside the Nginx directory (git --git-dir), so it
// never conflicts with etckeeper or a repository the admin already keeps
// in /etc/nginx, and Nginx never sees a .git directory.
package history

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/nginx"
)

// ConfigTester is satisfied by *nginx.Validator.
type ConfigTester interface {
	Test(ctx context.Context) (nginx.Result, error)
}

// Commit is one entry of the history.
type Commit struct {
	Hash    string
	Time    time.Time
	Message string
}

// History tracks Paths (relative to WorkTree) in the repository at GitDir.
type History struct {
	GitDir   string
	WorkTree string
	Paths    []string
}

// Available reports whether git is installed.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func (h *History) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--git-dir=" + h.GitDir, "--work-tree=" + h.WorkTree}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// init creates the repository on first use.
func (h *History) init(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(h.GitDir, "HEAD")); err == nil {
		return nil
	}
	if err := os.MkdirAll(h.GitDir, 0o700); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "GoNix"},
		{"config", "user.email", "gonix@localhost"},
		{"config", "core.symlinks", "true"},
	} {
		if _, err := h.git(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

// existingPaths filters Paths to those present, since git rejects pathspecs
// that match nothing. Paths that were tracked before are kept so their
// deletion is recorded too.
func (h *History) existingPaths(ctx context.Context) []string {
	var out []string
	for _, p := range h.Paths {
		if _, err := os.Lstat(filepath.Join(h.WorkTree, p)); err == nil {
			out = append(out, p)
			continue
		}
		if tracked, _ := h.git(ctx, "ls-files", "--", p); strings.TrimSpace(tracked) != "" {
			out = append(out, p)
		}
	}
	return out
}

// Commit records the current state of the tracked paths. It does nothing
// when nothing changed.
func (h *History) Commit(ctx context.Context, message string) error {
	if err := h.init(ctx); err != nil {
		return err
	}
	paths := h.existingPaths(ctx)
	if len(paths) == 0 {
		return nil
	}
	if _, err := h.git(ctx, append([]string{"add", "--all", "--"}, paths...)...); err != nil {
		return err
	}
	if out, _ := h.git(ctx, "status", "--porcelain", "--untracked-files=no"); strings.TrimSpace(out) == "" {
		return nil
	}
	_, err := h.git(ctx, "commit", "--quiet", "--no-verify", "-m", message)
	return err
}

// Log returns up to n most recent commits, newest first.
func (h *History) Log(ctx context.Context, n int) ([]Commit, error) {
	if _, err := os.Stat(filepath.Join(h.GitDir, "HEAD")); err != nil {
		return nil, nil
	}
	out, err := h.git(ctx, "log", fmt.Sprintf("-n%d", n), "--format=%H%x1f%ct%x1f%s")
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") {
			return nil, nil
		}
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 3 {
			continue
		}
		var sec int64
		fmt.Sscan(f[1], &sec)
		commits = append(commits, Commit{Hash: f[0], Time: time.Unix(sec, 0), Message: f[2]})
	}
	return commits, nil
}

// Show returns the commit's message and diff.
func (h *History) Show(ctx context.Context, hash string) (string, error) {
	if err := validHash(hash); err != nil {
		return "", err
	}
	return h.git(ctx, "show", "--stat", "--patch", "--format=%H%n%cd%n%n%s%n", hash)
}

// Restore rolls the tracked paths back to commit hash: files changed since
// are restored, files added since are removed. The current state is
// committed first, so a restore can itself be undone. If `nginx -t` rejects
// the result, the previous state is put back and an error returned.
func (h *History) Restore(ctx context.Context, hash string, tester ConfigTester) error {
	if err := validHash(hash); err != nil {
		return err
	}
	if err := h.Commit(ctx, "Before restoring "+short(hash)); err != nil {
		return err
	}
	head, err := h.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	head = strings.TrimSpace(head)
	if err := h.checkout(ctx, hash); err != nil {
		return err
	}
	if tester != nil {
		res, err := tester.Test(ctx)
		if err != nil || !res.OK {
			_ = h.checkout(ctx, head)
			if err != nil {
				return fmt.Errorf("testing configuration: %w", err)
			}
			return fmt.Errorf("nginx configuration test failed, nothing was restored:\n%s", res.Output)
		}
	}
	return h.Commit(ctx, "Restored "+short(hash))
}

func (h *History) checkout(ctx context.Context, hash string) error {
	paths := h.existingPaths(ctx)
	// Also include paths that only exist in the target commit.
	for _, p := range h.Paths {
		if out, _ := h.git(ctx, "ls-tree", "--name-only", hash, "--", p); strings.TrimSpace(out) != "" && !contains(paths, p) {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	_, err := h.git(ctx, append([]string{"restore", "--source=" + hash, "--staged", "--worktree", "--"}, paths...)...)
	return err
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func validHash(hash string) error {
	if len(hash) < 7 || len(hash) > 64 || strings.Trim(hash, "0123456789abcdef") != "" {
		return fmt.Errorf("%q is not a commit hash", hash)
	}
	return nil
}

func short(hash string) string {
	if len(hash) > 10 {
		return hash[:10]
	}
	return hash
}

// TrackedPaths converts absolute directories (sites-available, snippets,
// ...) into paths relative to workTree, dropping those outside it.
func TrackedPaths(workTree string, dirs ...string) []string {
	var out []string
	for _, d := range dirs {
		rel, err := filepath.Rel(workTree, d)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		if !contains(out, rel) {
			out = append(out, rel)
		}
	}
	return out
}

// Message is the commit message for an audited change, e.g.
// "host_created example.com (root via cli)".
func Message(e audit.Entry) string {
	by := e.User
	if e.Actor != "" {
		by += " via " + e.Actor
	}
	return fmt.Sprintf("%s %s (%s)", e.Action, e.Target, by)
}
