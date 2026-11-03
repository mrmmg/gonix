package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/history"
)

func TestHistoryRecordsAndRestores(t *testing.T) {
	if !history.Available() {
		t.Skip("git not installed")
	}
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}
	available := deps.Config.Nginx.SitesAvailable
	deps.History = &history.History{GitDir: filepath.Join(t.TempDir(), "h.git"), WorkTree: filepath.Dir(available),
		Paths: []string{filepath.Base(available)}}
	deps.Audit.OnSuccess = func(e audit.Entry) { _ = deps.History.Commit(backgroundCtx(), history.Message(e)) }

	createTestHost(t, deps, "example.com")
	h, _ := deps.Manager.GetHost("example.com")
	if _, err := deps.HostService.UpdateHost(backgroundCtx(), h); err != nil { // commit v1
		t.Fatal(err)
	}
	h.Locations[0].Proxy.UpstreamPort = 9999
	if _, err := deps.HostService.UpdateHost(backgroundCtx(), h); err != nil { // commit v2
		t.Fatal(err)
	}

	root := &rootModel{deps: deps}
	hs := newHistoryScreen(deps)
	root.stack = []screen{newMainMenu(deps), hs}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if len(hs.commits) != 2 {
		t.Fatalf("commits = %+v", hs.commits)
	}
	pressEnter(t, model) // diff of the newest commit
	if v, ok := topScreen(root).(*viewerScreen); !ok || !strings.Contains(strings.Join(v.lines, "\n"), "9999") {
		t.Fatalf("expected a diff mentioning the new port, got %T", topScreen(root))
	}

	hs = newHistoryScreen(deps)
	root.stack = []screen{newMainMenu(deps), hs}
	model = pressDown(t, model, 1)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	requireOKResult(t, root)
	data, _ := os.ReadFile(filepath.Join(available, "example.com"))
	if !strings.Contains(string(data), "127.0.0.1:8080") {
		t.Errorf("restored config:\n%s", data)
	}
}
