package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestStreamWizardCreatesProxy(t *testing.T) {
	deps := testDeps(t)
	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newStreamWizard(deps, nil)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	model = typeText(t, model, "dns")
	model = pressEnter(t, model)
	model = pressDown(t, model, 1) // udp
	model = pressEnter(t, model)
	model = typeText(t, model, "53")
	model = pressEnter(t, model)
	model = typeText(t, model, "10.0.0.53:53")
	model = pressEnter(t, model)
	model = pressEnter(t, model) // connect timeout
	model = pressEnter(t, model) // idle timeout
	model = pressEnter(t, model) // no IP list
	requireOKResult(t, root)

	p, err := deps.Streams.Get("dns")
	if err != nil || !p.UDP || p.Port != 53 || p.Upstream != "10.0.0.53:53" || !p.Enabled {
		t.Fatalf("proxy = %+v, %v", p, err)
	}
	_ = newStreamsScreen(deps).View(100, 40)
}
