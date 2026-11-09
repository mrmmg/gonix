// Package update finds out whether a newer GoNix has been released, and
// replaces the running binary with it.
//
// The only source is this project's GitHub Releases (Repo): the release
// information comes from the GitHub API, and the binary and its checksum
// from the release's own download URLs, always over HTTPS. Nothing else is
// ever contacted or trusted; a download whose SHA-256 does not match the
// release's checksum file is thrown away.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub repository GoNix is released from.
const Repo = "mrmmg/gonix"

// ReleasesURL is the page of a release is under, for people to read.
const ReleasesURL = "https://github.com/" + Repo + "/releases"

const (
	apiLatest    = "https://api.github.com/repos/" + Repo + "/releases/latest"
	downloadBase = "https://github.com/" + Repo + "/releases/download"
	maxBinary    = 200 << 20 // a GoNix binary is a few tens of MB
	maxNotes     = 2000
)

// Release is a published GoNix version.
type Release struct {
	Tag       string    `json:"tag"`     // e.g. v1.2.3
	Version   string    `json:"version"` // e.g. 1.2.3
	Notes     string    `json:"notes"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
}

var reTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// Client talks to GitHub. The zero fields of NewClient are the real thing;
// tests point API and Base at a local server.
type Client struct {
	HTTP *http.Client
	API  string
	Base string
}

// NewClient returns a Client for Repo. Redirects are only followed over
// HTTPS to GitHub's own hosts (release downloads are redirected to
// githubusercontent.com).
func NewClient() *Client {
	return &Client{
		API: apiLatest, Base: downloadBase,
		HTTP: &http.Client{
			Timeout: 5 * time.Minute,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 5 {
					return errors.New("too many redirects")
				}
				if !githubHost(req.URL) {
					return fmt.Errorf("refusing to follow a redirect to %s", req.URL.Host)
				}
				return nil
			},
		},
	}
}

func githubHost(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && (h == "github.com" || strings.HasSuffix(h, ".github.com") ||
		h == "githubusercontent.com" || strings.HasSuffix(h, ".githubusercontent.com"))
}

func (c *Client) get(ctx context.Context, rawURL string, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gonix-updater")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub answered %s for %s", resp.Status, req.URL.Redacted())
	}
	return resp, nil
}

// Latest asks GitHub for the latest published release.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := c.get(ctx, c.API, "application/vnd.github+json")
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	var raw struct {
		Tag        string    `json:"tag_name"`
		Body       string    `json:"body"`
		URL        string    `json:"html_url"`
		Published  time.Time `json:"published_at"`
		Draft      bool      `json:"draft"`
		Prerelease bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return Release{}, fmt.Errorf("reading the release information: %w", err)
	}
	if !reTag.MatchString(raw.Tag) {
		return Release{}, fmt.Errorf("the latest release has an unexpected tag %q", raw.Tag)
	}
	notes := strings.TrimSpace(raw.Body)
	if len(notes) > maxNotes {
		notes = notes[:maxNotes] + "…"
	}
	rel := Release{Tag: raw.Tag, Version: strings.TrimPrefix(raw.Tag, "v"), Notes: notes, Published: raw.Published,
		URL: ReleasesURL + "/tag/" + raw.Tag}
	return rel, nil
}

func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// Newer reports whether latest is a higher version than current. A current
// version that is not a plain x.y.z (a development build) is never
// considered out of date.
func Newer(latest, current string) bool {
	l, ok1 := parseVersion(latest)
	c, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// --- the cache of the last check ---

// CacheTTL is how long a check's result is reused.
const CacheTTL = 24 * time.Hour

type cacheEntry struct {
	Checked time.Time `json:"checked"`
	Release Release   `json:"release"`
}

// ReadCache returns the release found by the last check, and whether that
// check is recent enough (fresh) to be reused.
func ReadCache(path string) (rel Release, fresh, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Release{}, false, false
	}
	var e cacheEntry
	if json.Unmarshal(data, &e) != nil || !reTag.MatchString(e.Release.Tag) {
		return Release{}, false, false
	}
	return e.Release, time.Since(e.Checked) < CacheTTL, true
}

func writeCache(path string, rel Release) {
	if data, err := json.Marshal(cacheEntry{Checked: time.Now(), Release: rel}); err == nil {
		if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
			_ = os.WriteFile(path, data, 0o644)
		}
	}
}

// Check returns the latest release. Unless force is set, a result from the
// last 24 hours (see ReadCache) is reused instead of asking GitHub again.
func (c *Client) Check(ctx context.Context, cachePath string, force bool) (Release, error) {
	if !force && cachePath != "" {
		if rel, fresh, ok := ReadCache(cachePath); ok && fresh {
			return rel, nil
		}
	}
	rel, err := c.Latest(ctx)
	if err != nil {
		return Release{}, err
	}
	if cachePath != "" {
		writeCache(cachePath, rel)
	}
	return rel, nil
}

// --- replacing the binary ---

// BackupPath is where the previous binary is kept.
func BackupPath(exe string) string { return exe + ".old" }

// CheckExecutable refuses to update a binary that is not an installed
// release: one run with `go run`, or in a directory that cannot be written.
func CheckExecutable(exe string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("self-update supports Linux only")
	}
	if strings.Contains(exe, "go-build") {
		return fmt.Errorf("%s is a temporary build (go run); update an installed gonix instead", exe)
	}
	f, err := os.CreateTemp(filepath.Dir(exe), ".gonix-write-test-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (run as root?): %w", filepath.Dir(exe), err)
	}
	f.Close()
	return os.Remove(f.Name())
}

func assetName() (string, error) {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return "gonix-linux-" + runtime.GOARCH, nil
	}
	return "", fmt.Errorf("no release is built for %s", runtime.GOARCH)
}

// Apply downloads release rel, checks it against the release's SHA-256
// checksum file, makes sure it runs, keeps the current binary as exe+".old"
// (replacing any older backup) and replaces exe with it. progress, if set,
// is told what is happening. Until the final rename nothing at exe changes,
// so a failure at any step leaves the installed GoNix as it was.
func (c *Client) Apply(ctx context.Context, exe string, rel Release, progress func(string)) error {
	say := func(s string) {
		if progress != nil {
			progress(s)
		}
	}
	if !reTag.MatchString(rel.Tag) {
		return fmt.Errorf("invalid release tag %q", rel.Tag)
	}
	if err := CheckExecutable(exe); err != nil {
		return err
	}
	name, err := assetName()
	if err != nil {
		return err
	}
	assetURL := c.Base + "/" + rel.Tag + "/" + name

	say("Downloading the checksum of " + rel.Tag + "…")
	want, err := c.fetchChecksum(ctx, assetURL+".sha256", name)
	if err != nil {
		return err
	}

	say("Downloading " + name + "…")
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".gonix-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once it has been renamed
	got, err := c.download(ctx, assetURL, tmp)
	tmp.Close()
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("checksum mismatch: the download is %s, the release says %s; nothing was changed", got, want)
	}
	say("Checksum verified (SHA-256 " + got[:16] + "…).")

	mode := os.FileMode(0o755)
	if st, err := os.Stat(exe); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, tmp.Name(), "--version").CombinedOutput()
	if err != nil || !strings.Contains(string(out), rel.Version) {
		return fmt.Errorf("the downloaded binary does not run as version %s (%v: %s); nothing was changed", rel.Version, err, strings.TrimSpace(string(out)))
	}

	say("Keeping the current version as " + BackupPath(exe) + "…")
	if err := copyFile(exe, BackupPath(exe)); err != nil {
		return fmt.Errorf("saving the current version: %w", err)
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		return fmt.Errorf("replacing %s: %w", exe, err)
	}
	return nil
}

// Rollback puts the binary kept by the last update back. The version it
// replaces becomes the new backup, so rolling back twice returns to where
// it started.
func Rollback(exe string) error {
	if err := CheckExecutable(exe); err != nil {
		return err
	}
	old, swap := BackupPath(exe), exe+".swap"
	if _, err := os.Stat(old); err != nil {
		return fmt.Errorf("there is no previous version at %s", old)
	}
	if err := os.Rename(exe, swap); err != nil {
		return err
	}
	if err := os.Rename(old, exe); err != nil {
		_ = os.Rename(swap, exe)
		return err
	}
	return os.Rename(swap, old)
}

func (c *Client) fetchChecksum(ctx context.Context, rawURL, asset string) (string, error) {
	resp, err := c.get(ctx, rawURL, "")
	if err != nil {
		return "", fmt.Errorf("downloading the checksum: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	// "<sha256>  <file name>", as written by sha256sum.
	f := strings.Fields(string(data))
	if len(f) == 0 || len(f[0]) != 64 {
		return "", fmt.Errorf("the checksum file is not a SHA-256 sum")
	}
	if _, err := hex.DecodeString(f[0]); err != nil {
		return "", fmt.Errorf("the checksum file is not a SHA-256 sum")
	}
	if len(f) > 1 && strings.TrimPrefix(f[1], "*") != asset {
		return "", fmt.Errorf("the checksum file is for %s, not %s", f[1], asset)
	}
	return strings.ToLower(f[0]), nil
}

// download copies rawURL into w and returns the SHA-256 of what it copied.
func (c *Client) download(ctx context.Context, rawURL string, w io.Writer) (string, error) {
	resp, err := c.get(ctx, rawURL, "application/octet-stream")
	if err != nil {
		return "", fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(resp.Body, maxBinary+1))
	if err != nil {
		return "", fmt.Errorf("downloading: %w", err)
	}
	if n > maxBinary {
		return "", fmt.Errorf("the download is larger than %d MB; refusing it", maxBinary>>20)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile copies src to dst through a temporary file, keeping src's mode.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".gonix-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), st.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// CachePath is where the last check is remembered, next to the audit log
// (which is per instance when the instance has its own).
func CachePath(auditFile string) string {
	return filepath.Join(filepath.Dir(auditFile), "update-check.json")
}
