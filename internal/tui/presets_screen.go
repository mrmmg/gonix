package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/presets"
)

// newHostPresetsWizard asks one Yes/No question per preset that applies to
// the host, prefilled with whether it is currently enabled.
func newHostPresetsWizard(deps Deps, h nginx.ListedHost) screen {
	back := func() screen { return newHostDetailScreen(deps, h) }
	host, err := deps.Manager.GetHost(h.FileName)
	if err != nil {
		return newResultScreen(deps, "Presets", false, err.Error(), back())
	}
	enabled := map[string]bool{}
	for _, p := range host.Presets {
		enabled[p] = true
	}
	applicable := presets.Applicable(host)
	var fields []wizardField
	for _, p := range applicable {
		fields = append(fields, wizardField{
			Key: p.Name, Label: p.Title + "? — " + p.Description, Kind: fieldBool, BoolDefault: enabled[p.Name],
		})
	}

	return newWizard("Presets — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Presets", false, err.Error(), back()), nil
		}
		var selected []string
		for _, p := range applicable {
			if v[p.Name] == "yes" {
				selected = append(selected, p.Name)
			}
		}
		// Snippets must exist before the host includes them.
		if err := deps.Presets.Ensure(backgroundCtx(), selected); err != nil {
			return newResultScreen(deps, "Presets", false, err.Error(), back()), nil
		}
		host.Presets = presets.Sort(selected)
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Presets", false, err.Error(), back()), nil
		}
		msg := "Presets updated for " + h.ServerName + "."
		for _, name := range selected {
			if p, _ := presets.Get(name); p.HTTP != "" {
				msg += includeWarning(deps)
				break
			}
		}
		return newResultScreen(deps, "Presets", true, msg, back()), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}

func presetsDesc(h nginx.ListedHost) string {
	if len(h.Presets) == 0 {
		return mutedStyle.Render("(none)")
	}
	return strings.Join(h.Presets, ", ")
}
