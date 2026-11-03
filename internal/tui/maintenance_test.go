package tui

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMaintenanceWizardOnAndOff(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}
	t.Setenv("SSH_CLIENT", "203.0.113.5 52000 22")
	createTestHost(t, deps, "example.com")
	listed, _ := deps.Manager.ListHosts()

	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newMaintenanceWizard(deps, listed[0])}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if got := root.stack[1].(*wizardScreen).input.Value(); got != "203.0.113.5" {
		t.Errorf("bypass prefill = %q, want the SSH client address", got)
	}
	model = pressEnter(t, model)
	requireOKResult(t, root)

	h, _ := deps.Manager.GetHost("example.com")
	if !h.Maintenance || !reflect.DeepEqual(deps.Maintenance.Bypass("example.com"), []string{"203.0.113.5"}) {
		t.Fatalf("maintenance=%v bypass=%v", h.Maintenance, deps.Maintenance.Bypass("example.com"))
	}

	listed, _ = deps.Manager.ListHosts()
	root.stack = []screen{newMainMenu(deps), newMaintenanceWizard(deps, listed[0])}
	model = pressEnter(t, model) // Turn maintenance mode off
	requireOKResult(t, root)
	h, _ = deps.Manager.GetHost("example.com")
	if h.Maintenance {
		t.Error("maintenance mode still on")
	}
}
