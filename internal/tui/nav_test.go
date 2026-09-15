package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
)

// createTestHost writes a minimal managed host into deps' sites-available
// directory, matching what Add New Host would have produced, so navigation
// tests have something real to drill into.
func createTestHost(t *testing.T, deps Deps, name string) {
	t.Helper()
	h := nginx.Host{
		ServerName: name,
		Mode:       nginx.ModeReverseProxy,
		Listen:     80,
		AccessLog:  true,
		ErrorLog:   true,
		Locations: []nginx.Location{
			{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}},
		},
	}
	if err := deps.Manager.WriteHost(h); err != nil {
		t.Fatalf("WriteHost: %v", err)
	}
}

func pressEsc(t *testing.T, model tea.Model) tea.Model {
	t.Helper()
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	for cmd != nil {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	return model
}

func pressEnter(t *testing.T, model tea.Model) tea.Model {
	t.Helper()
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		model, cmd = model.Update(msg)
	}
	return model
}

// TestEscFromBackupRestoreReturnsToHostDetail is a regression test for a bug
// where selecting "Backup / Restore" from a host's submenu, then pressing
// Esc, skipped past the host detail screen straight back to the hosts list.
// This happened because the host detail screen was replaced in place (not
// pushed) when navigating to Backup/Restore, so Backup/Restore's Esc
// handler (which called navPop) ended up popping the hosts list screen
// instead of revealing host detail.
func TestEscFromBackupRestoreReturnsToHostDetail(t *testing.T) {
	deps := testDeps(t)
	createTestHost(t, deps, "example.com")

	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	// Main Menu -> Manage Hosts.
	root.stack[len(root.stack)-1].(*mainMenu).menu.cursor = miManageHosts
	model = pressEnter(t, model)

	// Manage Hosts -> select the (only) host.
	hostsScreen := root.stack[len(root.stack)-1].(*hostsListScreen)
	if len(hostsScreen.hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hostsScreen.hosts))
	}
	model = pressEnter(t, model)

	if _, ok := root.stack[len(root.stack)-1].(*hostDetailScreen); !ok {
		t.Fatalf("expected hostDetailScreen after selecting host, got %T", root.stack[len(root.stack)-1])
	}

	// Host Detail -> Backup / Restore.
	root.stack[len(root.stack)-1].(*hostDetailScreen).menu.cursor = hdBackupRestore
	model = pressEnter(t, model)
	if _, ok := root.stack[len(root.stack)-1].(*backupScreen); !ok {
		t.Fatalf("expected backupScreen, got %T", root.stack[len(root.stack)-1])
	}

	// Esc from Backup/Restore must return to Host Detail, not Hosts List.
	model = pressEsc(t, model)
	if _, ok := root.stack[len(root.stack)-1].(*hostDetailScreen); !ok {
		t.Fatalf("expected Esc from backup screen to return to hostDetailScreen, got %T", root.stack[len(root.stack)-1])
	}

	// And Esc from there must still be able to reach Hosts List, then Main
	// Menu — i.e. the rest of the navigation stack survived.
	model = pressEsc(t, model)
	if _, ok := root.stack[len(root.stack)-1].(*hostsListScreen); !ok {
		t.Fatalf("expected Esc from hostDetailScreen to return to hostsListScreen, got %T", root.stack[len(root.stack)-1])
	}
	model = pressEsc(t, model)
	if _, ok := root.stack[len(root.stack)-1].(*mainMenu); !ok {
		t.Fatalf("expected Esc from hostsListScreen to return to mainMenu, got %T", root.stack[len(root.stack)-1])
	}
}

// TestNavReplacePreservesStackBelow is a regression test for a bug where
// finishing a wizard (e.g. SSL Configuration) and dismissing its result
// screen wiped the *entire* navigation stack down to a single screen,
// leaving Esc permanently non-functional (there was nothing left to pop
// back to below the one remaining screen).
func TestNavReplacePreservesStackBelow(t *testing.T) {
	deps := testDeps(t)
	createTestHost(t, deps, "example.com")

	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	root.stack[len(root.stack)-1].(*mainMenu).menu.cursor = miManageHosts
	model = pressEnter(t, model)
	model = pressEnter(t, model) // select the host

	if len(root.stack) != 3 {
		t.Fatalf("expected stack depth 3 (main, hosts list, host detail), got %d", len(root.stack))
	}

	// Simulate a wizard finishing and handing off to a result screen via
	// navReplace, as every host-detail wizard does on completion.
	fresh := newHostDetailScreen(deps, root.stack[len(root.stack)-1].(*hostDetailScreen).host)
	result := newResultScreen(deps, "Test", true, "done", fresh)
	root.stack[len(root.stack)-1] = result
	model = pressEnter(t, model) // dismiss the result screen

	if got := len(root.stack); got != 3 {
		t.Fatalf("expected navReplace to preserve stack depth at 3, got %d", got)
	}
	if _, ok := root.stack[len(root.stack)-1].(*hostDetailScreen); !ok {
		t.Fatalf("expected hostDetailScreen on top after dismissing result, got %T", root.stack[len(root.stack)-1])
	}

	// Esc must still be able to walk all the way back out.
	model = pressEsc(t, model)
	if _, ok := root.stack[len(root.stack)-1].(*hostsListScreen); !ok {
		t.Fatalf("expected hostsListScreen, got %T", root.stack[len(root.stack)-1])
	}
	model = pressEsc(t, model)
	if _, ok := root.stack[len(root.stack)-1].(*mainMenu); !ok {
		t.Fatalf("expected mainMenu, got %T", root.stack[len(root.stack)-1])
	}
}
