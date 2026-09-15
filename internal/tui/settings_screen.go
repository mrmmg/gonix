package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// settingsScreen shows the effective nginx-manager configuration. Editing is
// intentionally done by hand in the YAML file (see internal/config) rather
// than through the TUI, since these are low-frequency, high-impact settings
// best reviewed in a text editor / version control.
type settingsScreen struct {
	deps Deps
}

func newSettingsScreen(deps Deps) *settingsScreen { return &settingsScreen{deps: deps} }

func (s *settingsScreen) Init() tea.Cmd { return nil }

func (s *settingsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc", "q", "enter":
			return s, navPop()
		}
	}
	return s, nil
}

func (s *settingsScreen) View(width, height int) string {
	c := s.deps.Config
	body := headerStyle.Render("Settings") + "\n\n"
	body += mutedStyle.Render("Edit /etc/nginx-manager/nginx-manager.yaml and restart to change these.") + "\n\n"
	body += fmt.Sprintf("Nginx config dir:       %s\n", c.Nginx.ConfigDir)
	body += fmt.Sprintf("sites-available:        %s\n", c.Nginx.SitesAvailable)
	body += fmt.Sprintf("sites-enabled:          %s\n", c.Nginx.SitesEnabled)
	body += fmt.Sprintf("nginx binary:           %s\n", c.Nginx.BinaryPath)
	body += fmt.Sprintf("Certificates directory: %s\n", c.Certificates.Directory)
	body += fmt.Sprintf("Nginx log directory:    %s\n", c.Logs.NginxDirectory)
	body += fmt.Sprintf("Audit log file:         %s\n", c.Logs.AuditFile)
	body += fmt.Sprintf("Backup directory:       %s (keep %d)\n", c.Backup.Directory, c.Backup.KeepCount)
	body += fmt.Sprintf("Access lists directory: %s\n", c.AccessLists.Directory)
	help := [][2]string{{"Esc", "Back"}}
	return renderFrame(width, height, "NGINX MANAGER", body, help)
}
