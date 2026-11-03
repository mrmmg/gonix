package tui

import (
	"os"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHostPresetsWizard(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}
	createTestHost(t, deps, "example.com")
	listed, _ := deps.Manager.ListHosts()

	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newHostPresetsWizard(deps, listed[0])}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	// No SSL: security-headers, gzip, static-cache, proxy-cache,
	// block-sensitive-files, block-bad-bots are offered (HSTS is not).
	// Enable the first two.
	model = pressDown(t, model, 1)
	model = pressEnter(t, model)
	model = pressDown(t, model, 1)
	model = pressEnter(t, model)
	for i := 0; i < 4; i++ {
		model = pressEnter(t, model)
	}
	requireOKResult(t, root)

	h, err := deps.Manager.GetHost("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"security-headers", "gzip"}; !reflect.DeepEqual(h.Presets, want) {
		t.Errorf("Presets = %v, want %v", h.Presets, want)
	}
	if _, err := os.Stat(deps.Presets.ServerPath("gzip")); err != nil {
		t.Errorf("preset snippet not written: %v", err)
	}
}
