package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/geoblock"
)

func TestCountryBlockingFlow(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = okTester{}
	deps.GeoBlock.Database = t.TempDir() + "/GeoLite2-Country.mmdb"
	os.WriteFile(deps.GeoBlock.Database, []byte("fake"), 0o644)
	createTestHost(t, deps, "example.com")

	root := &rootModel{deps: deps}
	root.stack = []screen{newMainMenu(deps), newGeoScreen(deps)}
	var model tea.Model = root
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	model = pressEnter(t, model) // Set Up GeoIP
	requireOKResult(t, root)

	root.stack = []screen{newMainMenu(deps), newCountryListWizard(deps, nil)}
	model = typeText(t, model, "no_cn")
	model = pressEnter(t, model)
	model = pressEnter(t, model) // blocklist
	model = typeText(t, model, "cn")
	model = pressEnter(t, model)
	requireOKResult(t, root)
	if l, err := deps.GeoBlock.Get("no_cn"); err != nil || l.Mode != geoblock.Blocklist || l.Countries[0] != "CN" {
		t.Fatalf("list = %+v, %v", l, err)
	}

	listed, _ := deps.Manager.ListHosts()
	root.stack = []screen{newMainMenu(deps), newHostCountryWizard(deps, listed[0])}
	model = pressDown(t, model, 1)
	pressEnter(t, model)
	requireOKResult(t, root)
	if h, _ := deps.Manager.GetHost("example.com"); h.CountryList != "no_cn" {
		t.Errorf("CountryList = %q", h.CountryList)
	}
}
