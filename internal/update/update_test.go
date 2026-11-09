package update

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, cur string
		want        bool
	}{
		{"v1.2.0", "1.1.9", true}, {"1.10.0", "1.9.0", true}, {"v1.0.0", "1.0.0", false},
		{"v1.0.0", "1.0.1", false}, {"v1.0.0", "dev", false}, {"garbage", "1.0.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.cur); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.latest, c.cur, got, c.want)
		}
	}
}

// fakeGitHub serves a release whose binary is a script printing its version.
func fakeGitHub(t *testing.T, version string, tamper bool) (*Client, *int) {
	t.Helper()
	script := "#!/bin/sh\necho \"gonix version " + version + "\"\n"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(script)))
	name := "gonix-linux-" + runtime.GOARCH
	hits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprintf(w, `{"tag_name":"v%s","body":"notes here","published_at":"2026-01-01T00:00:00Z"}`, version)
	})
	mux.HandleFunc("/dl/v"+version+"/"+name, func(w http.ResponseWriter, r *http.Request) {
		if tamper {
			script += "# changed after the checksum was made\n"
		}
		fmt.Fprint(w, script)
	})
	mux.HandleFunc("/dl/v"+version+"/"+name+".sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", sum, name)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), API: srv.URL + "/api", Base: srv.URL + "/dl"}, &hits
}

func installed(t *testing.T, content string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "gonix")
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestCheckUsesTheCache(t *testing.T) {
	c, hits := fakeGitHub(t, "2.0.0", false)
	cache := filepath.Join(t.TempDir(), "sub", "update.json")
	for i := 0; i < 3; i++ {
		rel, err := c.Check(context.Background(), cache, false)
		if err != nil || rel.Tag != "v2.0.0" || rel.Notes != "notes here" {
			t.Fatalf("Check = %+v, %v", rel, err)
		}
	}
	if *hits != 1 {
		t.Errorf("GitHub was asked %d times, want 1", *hits)
	}
	if _, err := c.Check(context.Background(), cache, true); err != nil || *hits != 2 {
		t.Errorf("force should ask again (hits=%d, err=%v)", *hits, err)
	}
}

func TestApplyVerifiesBacksUpAndReplaces(t *testing.T) {
	c, _ := fakeGitHub(t, "2.0.0", false)
	exe := installed(t, "#!/bin/sh\necho \"gonix version 1.0.0\"\n")
	rel, _ := c.Latest(context.Background())
	var said []string
	if err := c.Apply(context.Background(), exe, rel, func(s string) { said = append(said, s) }); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if b, _ := os.ReadFile(exe); !strings.Contains(string(b), "2.0.0") {
		t.Errorf("exe was not replaced: %s", b)
	}
	if b, _ := os.ReadFile(BackupPath(exe)); !strings.Contains(string(b), "1.0.0") {
		t.Errorf("the previous version was not kept: %s", b)
	}
	if st, _ := os.Stat(exe); st.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v", st.Mode())
	}
	if !strings.Contains(strings.Join(said, "\n"), "Checksum verified") {
		t.Errorf("progress = %v", said)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".gonix-*"))
	if len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}

	// Rolling back swaps the two; rolling back again returns.
	if err := Rollback(exe); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); !strings.Contains(string(b), "1.0.0") {
		t.Errorf("rollback did not restore 1.0.0: %s", b)
	}
	if b, _ := os.ReadFile(BackupPath(exe)); !strings.Contains(string(b), "2.0.0") {
		t.Errorf("the rolled-back version should be the backup: %s", b)
	}
}

func TestApplyRefusesAChecksumMismatchAndChangesNothing(t *testing.T) {
	c, _ := fakeGitHub(t, "2.0.0", true)
	original := "#!/bin/sh\necho \"gonix version 1.0.0\"\n"
	exe := installed(t, original)
	rel, _ := c.Latest(context.Background())
	err := c.Apply(context.Background(), exe, rel, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want a checksum mismatch", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != original {
		t.Error("the installed binary was changed")
	}
	if _, err := os.Stat(BackupPath(exe)); err == nil {
		t.Error("a backup was made for a failed update")
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".gonix-update-*"))
	if len(left) != 0 {
		t.Errorf("the rejected download was left behind: %v", left)
	}
}

func TestOnlyGitHubIsFollowed(t *testing.T) {
	c := NewClient()
	for url, ok := range map[string]bool{
		"https://github.com/x":                          true,
		"https://objects.githubusercontent.com/x":       true,
		"https://release-assets.githubusercontent.com/": true,
		"http://github.com/x":                           false,
		"https://evilgithub.com/x":                      false,
		"https://github.com.evil.io/x":                  false,
	} {
		req, _ := http.NewRequest("GET", url, nil)
		if err := c.HTTP.CheckRedirect(req, nil); (err == nil) != ok {
			t.Errorf("redirect to %s: err=%v, allowed want %v", url, err, ok)
		}
	}
}

func TestRefusesATemporaryBuild(t *testing.T) {
	if err := CheckExecutable("/tmp/go-build123/b001/exe/gonix"); err == nil {
		t.Error("go run builds must not be updated")
	}
}
