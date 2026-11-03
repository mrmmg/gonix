package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/ratelimit"
)

func TestRateLimitWizardsCreateAndApply(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}
	createTestHost(t, deps, "example.com")

	// Create from the "api" preset.
	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newRateLimitWizard(deps, nil)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	model = pressDown(t, model, 1) // preset: api
	model = pressEnter(t, model)
	model = typeText(t, model, "api")
	model = pressEnter(t, model)
	requireOKResult(t, root)

	got, err := deps.RateLimits.Get("api")
	want := ratelimit.Presets[1]
	want.Name = "api"
	if err != nil || got != want {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, want)
	}

	// Create a custom one: 3 requests per minute, no burst.
	root.stack = []screen{newMainMenu(deps), newRateLimitWizard(deps, nil)}
	model = pressDown(t, model, len(ratelimit.Presets)) // Custom
	model = pressEnter(t, model)
	model = typeText(t, model, "strict")
	model = pressEnter(t, model)
	root.stack[len(root.stack)-1].(*wizardScreen).input.SetValue("3")
	model = pressEnter(t, model)
	model = pressDown(t, model, 1) // per minute
	model = pressEnter(t, model)
	root.stack[len(root.stack)-1].(*wizardScreen).input.SetValue("0")
	model = pressEnter(t, model)
	model = pressEnter(t, model) // nodelay (ignored without burst)
	root.stack[len(root.stack)-1].(*wizardScreen).input.SetValue("0")
	model = pressEnter(t, model)
	requireOKResult(t, root)
	strict, err := deps.RateLimits.Get("strict")
	if err != nil || strict.Rate != 3 || strict.Per != "m" || strict.Burst != 0 || strict.Conn != 0 {
		t.Fatalf("strict = %+v, %v", strict, err)
	}

	// Apply "api" to the whole host.
	listed, _ := deps.Manager.ListHosts()
	root.stack = []screen{newMainMenu(deps), newHostRateLimitWizard(deps, listed[0])}
	model = pressEnter(t, model)   // whole host
	model = pressDown(t, model, 1) // api
	model = pressEnter(t, model)
	requireOKResult(t, root)
	h, _ := deps.Manager.GetHost("example.com")
	if h.RateLimit != "api" {
		t.Errorf("host RateLimit = %q, want api", h.RateLimit)
	}
}
