package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/nginx"
)

const (
	maintTurnOff = "Turn maintenance mode off"
	maintEdit    = "Change bypass addresses"
)

// newMaintenanceWizard turns maintenance mode on (asking for bypass
// addresses) or, when it is already on, off or reconfigures it.
func newMaintenanceWizard(deps Deps, h nginx.ListedHost) *wizardScreen {
	back := func() screen { return newHostDetailScreen(deps, h) }
	bypass := strings.Join(deps.Maintenance.Bypass(h.ServerName), " ")
	if !h.Maintenance && bypass == "" {
		bypass = sshClientIP()
	}

	var fields []wizardField
	if h.Maintenance {
		fields = append(fields, wizardField{
			Key: "action", Label: h.ServerName + " is in maintenance mode.", Kind: fieldChoice,
			Options: []string{maintTurnOff, maintEdit},
		})
	}
	fields = append(fields, wizardField{
		Key:   "bypass",
		Label: "Addresses that still see the live site (space separated, empty for none; prefilled with your SSH address):",
		Kind:  fieldText, Placeholder: "203.0.113.5 10.0.0.0/8", Default: bypass,
		Validate: func(v string) error {
			if strings.TrimSpace(v) == "" {
				return nil
			}
			_, err := iplist.ParseEntries(v)
			return err
		},
		ShowIf: func(v map[string]string) bool { return v["action"] != maintTurnOff },
	})

	return newWizard("Maintenance Mode — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		if v["action"] == maintTurnOff {
			if err := deps.Maintenance.Disable(backgroundCtx(), h.FileName); err != nil {
				return newResultScreen(deps, "Maintenance Mode", false, err.Error(), back()), nil
			}
			return newResultScreen(deps, "Maintenance Mode", true, h.ServerName+" is live again.", back()), nil
		}
		var entries []string
		if strings.TrimSpace(v["bypass"]) != "" {
			entries, _ = iplist.ParseEntries(v["bypass"])
		}
		if err := deps.Maintenance.Enable(backgroundCtx(), h.FileName, entries); err != nil {
			return newResultScreen(deps, "Maintenance Mode", false, err.Error()+includeWarning(deps), back()), nil
		}
		msg := h.ServerName + " now answers 503 Service Unavailable"
		if len(entries) > 0 {
			msg += ", except for " + strings.Join(entries, ", ")
		}
		msg += ".\n\nFor a custom page, give this host an error pages snippet that handles 503."
		return newResultScreen(deps, "Maintenance Mode", true, msg+includeWarning(deps)+realIPHint(deps), back()), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}

// sshClientIP returns the address of the SSH session GoNix runs in, so an
// admin does not lock themselves out of their own site.
func sshClientIP() string {
	for _, env := range []string{"SSH_CLIENT", "SSH_CONNECTION"} {
		if f := strings.Fields(os.Getenv(env)); len(f) > 0 && iplist.ValidateEntry(f[0]) == nil {
			return f[0]
		}
	}
	return ""
}

func maintenanceDesc(h nginx.ListedHost) string {
	if h.Maintenance {
		return dangerStyle.Render("ON")
	}
	return mutedStyle.Render("off")
}
