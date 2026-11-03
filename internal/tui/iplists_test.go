package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/nginx"
)

func topScreen(root *rootModel) screen { return root.stack[len(root.stack)-1] }

func requireOKResult(t *testing.T, root *rootModel) {
	t.Helper()
	rs, ok := topScreen(root).(*resultScreen)
	if !ok {
		t.Fatalf("expected a result screen, got %T", topScreen(root))
	}
	if !rs.ok {
		t.Fatalf("action failed: %s", rs.message)
	}
}

func TestIPListWizardAndHostRules(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}
	h := nginx.Host{
		ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80, AccessListName: "admins",
		Locations: []nginx.Location{
			{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}},
			{Path: "/admin/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 9090, HTTPVersion: "1.1"}},
		},
	}
	if err := deps.Manager.WriteHost(h); err != nil {
		t.Fatal(err)
	}

	// Create an allowlist through the wizard.
	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newIPListWizard(deps, nil)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	model = typeText(t, model, "office")
	model = pressEnter(t, model)
	model = pressEnter(t, model) // allowlist
	model = typeText(t, model, "192.168.1.0/24, 10.0.0.5")
	model = pressEnter(t, model)
	requireOKResult(t, root)

	l, err := deps.IPLists.Store.Get("office")
	if err != nil || l.Mode != iplist.Allowlist || len(l.Entries) != 2 {
		t.Fatalf("list = %+v, %v", l, err)
	}

	// Apply it to /admin/ only.
	listed, _ := deps.Manager.ListHosts()
	root.stack = []screen{newMainMenu(deps), newHostIPRulesWizard(deps, listed[0])}
	model = pressDown(t, model, 2) // scope: Path /admin/
	model = pressEnter(t, model)
	model = pressDown(t, model, 1) // list: office
	model = pressEnter(t, model)
	requireOKResult(t, root)

	got, err := deps.Manager.GetHost("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.IPListName != "" || got.Locations[1].IPListName != "office" || got.Locations[0].IPListName != "" {
		t.Errorf("IP list applied to the wrong scope: host=%q locations=%q,%q",
			got.IPListName, got.Locations[0].IPListName, got.Locations[1].IPListName)
	}

	// Host-level with Basic Auth asks whether either is enough.
	listed, _ = deps.Manager.ListHosts()
	root.stack = []screen{newMainMenu(deps), newHostIPRulesWizard(deps, listed[0])}
	model = pressEnter(t, model)   // scope: whole host
	model = pressDown(t, model, 1) // list: office
	model = pressEnter(t, model)
	model = pressDown(t, model, 1) // satisfy any: yes
	model = pressEnter(t, model)
	requireOKResult(t, root)
	got, _ = deps.Manager.GetHost("example.com")
	if got.IPListName != "office" || !got.SatisfyAny {
		t.Errorf("host-level rules = %q satisfyAny=%v", got.IPListName, got.SatisfyAny)
	}

	// The list is in use, so it cannot be deleted.
	if err := deps.IPLists.Delete(backgroundCtx(), "office"); err == nil {
		t.Error("expected deleting a list in use to fail")
	}
}

func TestRealIPScreenEnablesCloudflare(t *testing.T) {
	deps := testDeps(t)
	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newRealIPScreen(deps)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = model.View()
	pressEnter(t, model) // Use Cloudflare
	requireOKResult(t, root)

	s, enabled, err := deps.RealIP.Get()
	if err != nil || !enabled || !s.Cloudflare || s.Header != "CF-Connecting-IP" {
		t.Errorf("settings = %+v enabled=%v err=%v", s, enabled, err)
	}
}
