package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/iplist"
)

func TestLiveLogsFilterAndBlock(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}
	deps.Config.Logs.NginxDirectory = t.TempDir()
	createTestHost(t, deps, "example.com")
	log := strings.Join([]string{
		`10.0.0.1 - - [03/Nov/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 10 "-" "a"`,
		`203.0.113.7 - - [03/Nov/2026:10:00:01 +0000] "GET /api HTTP/1.1" 502 10 "-" "a"`,
		`10.0.0.1 - - [03/Nov/2026:10:00:02 +0000] "GET /about HTTP/1.1" 200 10 "-" "a"`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(deps.Config.Logs.NginxDirectory, "example.com.access.log"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	listed, _ := deps.Manager.ListHosts()

	root := &rootModel{deps: deps}
	logsScreen := newLiveLogsScreen(deps, listed[0])
	root.stack = []screen{newMainMenu(deps), logsScreen}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if len(logsScreen.entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(logsScreen.entries))
	}
	_ = model.View()

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = typeText(t, model, "status:5xx")
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(logsScreen.entries) != 1 || logsScreen.entries[0].IP != "203.0.113.7" {
		t.Fatalf("filtered entries = %+v", logsScreen.entries)
	}

	// Summary view renders.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !strings.Contains(model.View(), "Top clients") {
		t.Error("summary view not shown")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	requireOKResult(t, root)

	h, _ := deps.Manager.GetHost("example.com")
	l, err := deps.IPLists.Store.Get(blockedListName)
	if err != nil || h.IPListName != blockedListName || l.Mode != iplist.Blocklist || !l.Contains("203.0.113.7") {
		t.Fatalf("host list=%q, blocked=%+v, err=%v", h.IPListName, l, err)
	}

	// Dismissing the result returns to the live log screen.
	pressEnter(t, model)
	if _, ok := topScreen(root).(*liveLogsScreen); !ok {
		t.Errorf("expected the live log screen after the result, got %T", topScreen(root))
	}

	// A second block goes into the same list.
	if _, err := blockIP(deps, "example.com", "198.51.100.9"); err != nil {
		t.Fatal(err)
	}
	if l, _ := deps.IPLists.Store.Get(blockedListName); len(l.Entries) != 2 {
		t.Errorf("entries = %v", l.Entries)
	}
}

func TestBlockIPRefusesAllowlist(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}
	createTestHost(t, deps, "example.com")
	if err := deps.IPLists.Create(backgroundCtx(), iplist.List{Name: "office", Mode: iplist.Allowlist, Entries: []string{"10.0.0.0/8"}}); err != nil {
		t.Fatal(err)
	}
	h, _ := deps.Manager.GetHost("example.com")
	h.IPListName = "office"
	if _, err := deps.HostService.UpdateHost(backgroundCtx(), h); err != nil {
		t.Fatal(err)
	}
	if _, err := blockIP(deps, "example.com", "10.1.2.3"); err == nil || !strings.Contains(err.Error(), "admitted by the allowlist") {
		t.Errorf("err = %v", err)
	}
	if _, err := blockIP(deps, "example.com", "203.0.113.7"); err == nil || !strings.Contains(err.Error(), "already refused") {
		t.Errorf("err = %v", err)
	}
}

func TestConfigCheckScreenRenders(t *testing.T) {
	deps := testDeps(t)
	createTestHost(t, deps, "example.com")
	s := newConfigCheckScreen(deps)
	if v := s.View(120, 40); !strings.Contains(v, "Config Check") {
		t.Errorf("unexpected view:\n%s", v)
	}
}
