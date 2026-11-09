package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/update"
)

func TestUpdateCommand(t *testing.T) {
	script := "#!/bin/sh\necho \"gonix version 2.0.0\"\n"
	name := "gonix-linux-" + runtime.GOARCH
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v2.0.0","body":"shiny"}`)
	})
	mux.HandleFunc("/dl/v2.0.0/"+name, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, script) })
	mux.HandleFunc("/dl/v2.0.0/"+name+".sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%x  %s\n", sha256.Sum256([]byte(script)), name)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "gonix")
	os.WriteFile(exe, []byte("#!/bin/sh\necho \"gonix version 1.0.0\"\n"), 0o755)
	cfg := filepath.Join(dir, "gonix.yaml")
	os.WriteFile(cfg, []byte("logs:\n  audit_file: "+filepath.Join(dir, "audit.log")+"\nupdate:\n  restart_exporter: false\n"), 0o644)

	oldClient, oldExe, oldVer, oldRoot := newUpdateClient, executablePath, Version, RequireRoot
	defer func() { newUpdateClient, executablePath, Version, RequireRoot = oldClient, oldExe, oldVer, oldRoot }()
	newUpdateClient = func() *update.Client {
		return &update.Client{HTTP: srv.Client(), API: srv.URL + "/api", Base: srv.URL + "/dl"}
	}
	executablePath = func() (string, error) { return exe, nil }
	Version, RequireRoot = "1.0.0", false

	run := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := Run(append(args, "--config", cfg), &out, &errb, nil, cfg)
		return code, out.String() + errb.String()
	}

	code, out := run("update", "--check")
	if code != 0 || !strings.Contains(out, "2.0.0 is available") || !strings.Contains(out, "shiny") {
		t.Fatalf("--check: %d %s", code, out)
	}
	if b, _ := os.ReadFile(exe); strings.Contains(string(b), "2.0.0") {
		t.Fatal("--check must not change anything")
	}

	// Without --yes (and no terminal answer) it must not update.
	if code, _ := run("update", "--json"); code == 0 {
		t.Error("a JSON update without --yes should be refused")
	}

	code, out = run("update", "--yes")
	if code != 0 || !strings.Contains(out, "Checksum verified") || !strings.Contains(out, "Updated gonix 1.0.0 → 2.0.0") {
		t.Fatalf("update: %d %s", code, out)
	}
	if b, _ := os.ReadFile(exe); !strings.Contains(string(b), "2.0.0") {
		t.Error("exe not updated")
	}
	if b, _ := os.ReadFile(update.BackupPath(exe)); !strings.Contains(string(b), "1.0.0") {
		t.Error("previous version not kept")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "audit.log")); !strings.Contains(string(b), "gonix_updated") {
		t.Errorf("the update was not audited: %s", b)
	}

	Version = "2.0.0"
	if code, out := run("update"); code != 0 || !strings.Contains(out, "up to date") {
		t.Errorf("already current: %d %s", code, out)
	}
	if code, out := run("update", "--rollback", "--yes"); code != 0 || !strings.Contains(out, "Rolled back") {
		t.Errorf("rollback: %d %s", code, out)
	}
	if b, _ := os.ReadFile(exe); !strings.Contains(string(b), "1.0.0") {
		t.Error("rollback did not restore 1.0.0")
	}
}
