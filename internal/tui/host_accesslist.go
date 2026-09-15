package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
)

// newHostAccessListScreen lets the operator apply or remove a host-level
// access list (HTTP Basic Authentication) from the available lists.
func newHostAccessListScreen(deps Deps, h nginx.ListedHost) *wizardScreen {
	lists, _ := deps.AccessLists.List()
	options := []string{"(none)"}
	for _, l := range lists {
		options = append(options, l.Name)
	}

	fields := []wizardField{
		{Key: "list", Label: "Apply access list to " + h.ServerName + ":", Kind: fieldChoice, Options: options},
	}
	return newWizard("Access List — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Access List", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		if v["list"] == "(none)" {
			host.AccessListName = ""
		} else {
			host.AccessListName = v["list"]
		}
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Access List", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "Access List", true, "Access list updated for "+h.ServerName+".", newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}
