package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/upstreams"
)

func TestUpstreamGroupWizardAndAddHost(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}

	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newUpstreamWizard(deps, nil)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	model = typeText(t, model, "app")
	model = pressEnter(t, model)
	model = pressDown(t, model, 1) // least_conn
	model = pressEnter(t, model)
	model = typeText(t, model, "10.0.0.11:3000, 10.0.0.12:3000 backup")
	model = pressEnter(t, model)
	model = pressEnter(t, model) // max fails 3
	model = pressEnter(t, model) // fail timeout 10s
	requireOKResult(t, root)

	g, err := deps.Upstreams.Get("app")
	if err != nil || g.Method != upstreams.LeastConn || len(g.Servers) != 2 || !g.Servers[1].Backup || g.MaxFails != 3 || g.FailTimeout != "10s" {
		t.Fatalf("group = %+v, %v", g, err)
	}

	// Add a reverse proxy host that targets the group.
	root.stack = []screen{newMainMenu(deps), newAddHostWizard(deps)}
	model = typeText(t, model, "example.com")
	model = pressEnter(t, model)
	model = pressEnter(t, model) // no aliases
	model = pressEnter(t, model) // Reverse Proxy
	model = pressDown(t, model, 1)
	model = pressEnter(t, model) // Group: app
	for i := 0; i < 8; i++ {     // scheme .. send timeout, SSL no
		model = pressEnter(t, model)
	}
	requireOKResult(t, root)
	h, err := deps.Manager.GetHost("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if p := h.Locations[0].Proxy; p.UpstreamGroup != "app" || p.UpstreamHost != "" {
		t.Errorf("proxy = %+v", p)
	}
}
