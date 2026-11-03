package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
)

// okTester stands in for `nginx -t` so wizard tests do not depend on the
// system's Nginx configuration.
type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func typeText(t *testing.T, model tea.Model, text string) tea.Model {
	t.Helper()
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return model
}

func pressDown(t *testing.T, model tea.Model, n int) tea.Model {
	t.Helper()
	for i := 0; i < n; i++ {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	return model
}

func TestAddHostWizardCreatesRedirectHost(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}

	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newAddHostWizard(deps)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	model = typeText(t, model, "domain.net")
	model = pressEnter(t, model)
	model = typeText(t, model, "www.domain.net")
	model = pressEnter(t, model)
	model = pressDown(t, model, 2) // Host mode: Redirect
	model = pressEnter(t, model)
	model = typeText(t, model, "domain.com") // bare domain, normalized to https://
	model = pressEnter(t, model)
	model = pressEnter(t, model) // 301
	model = pressEnter(t, model) // keep path: yes (default)
	model = pressEnter(t, model) // SSL: no (default)

	if _, ok := root.stack[len(root.stack)-1].(*resultScreen); !ok {
		t.Fatalf("expected a result screen after the wizard, got %T", root.stack[len(root.stack)-1])
	}
	if rs := root.stack[len(root.stack)-1].(*resultScreen); !rs.ok {
		t.Fatalf("creating the host failed: %s", rs.message)
	}

	h, err := deps.Manager.GetHost("domain.net")
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	want := nginx.RedirectConfig{Target: "https://domain.com", Code: 301, PreservePath: true}
	if h.Mode != nginx.ModeRedirect || h.Redirect != want {
		t.Errorf("host = mode %q redirect %+v; want redirect %+v", h.Mode, h.Redirect, want)
	}
	if len(h.ServerAlias) != 1 || h.ServerAlias[0] != "www.domain.net" {
		t.Errorf("ServerAlias = %v", h.ServerAlias)
	}

	// The redirect host's submenu offers redirect settings and hides the
	// proxy-only actions.
	listed, err := deps.Manager.ListHosts()
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListHosts = %v, %v", listed, err)
	}
	detail := newHostDetailScreen(deps, listed[0])
	if !detail.selectAction(hdRedirect) {
		t.Error("redirect host should offer Redirect Configuration")
	}
	for _, a := range []int{hdAddPath, hdWebSocket, hdProxy, hdAccessList, hdErrorPages} {
		if detail.selectAction(a) {
			t.Errorf("redirect host should not offer action %d", a)
		}
	}

	// A reverse proxy host keeps its full menu and no redirect entry.
	createTestHost(t, deps, "example.com")
	proxyHost, _ := deps.Manager.ListHosts()
	for _, lh := range proxyHost {
		if lh.ServerName != "example.com" {
			continue
		}
		d := newHostDetailScreen(deps, lh)
		if d.selectAction(hdRedirect) || !d.selectAction(hdProxy) || !d.selectAction(hdAliases) {
			t.Error("reverse proxy host menu is wrong")
		}
	}
}
