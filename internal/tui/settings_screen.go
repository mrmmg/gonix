package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/config"
)

// settingsScreen shows the effective GoNix configuration, and lets the
// update options be switched here. Everything else is intentionally edited
// by hand in the YAML file (see internal/config) rather than through the
// TUI, since these are low-frequency, high-impact settings best reviewed in
// a text editor / version control.
type settingsScreen struct {
	deps   Deps
	menu   *simpleMenu
	status string // what the last change did, or why it failed
	failed bool
}

func newSettingsScreen(deps Deps) *settingsScreen {
	s := &settingsScreen{deps: deps}
	s.menu = newSimpleMenu(nil)
	s.refresh()
	return s
}

func onOff(b bool) string {
	if b {
		return successStyle.Render("On")
	}
	return mutedStyle.Render("Off")
}

func (s *settingsScreen) refresh() {
	u := s.deps.Config.Update
	s.menu.SetItems([]menuItem{
		{title: "Check for updates: " + onOff(u.Check), desc: "ask GitHub once a day for a new GoNix release"},
		{title: "Restart the monitoring exporter after an update: " + onOff(u.RestartExporter), desc: "so it runs the new version"},
	})
}

// toggle flips an update option and saves it to the configuration file; the
// running GoNix uses it at once.
func (s *settingsScreen) toggle(i int) {
	u := s.deps.Config.Update
	if i == 0 {
		u.Check = !u.Check
	} else {
		u.RestartExporter = !u.RestartExporter
	}
	if s.deps.ConfigPath == "" {
		s.status, s.failed = "no configuration file is in use", true
		return
	}
	if err := config.SaveUpdate(s.deps.ConfigPath, u); err != nil {
		s.status, s.failed = err.Error(), true
		return
	}
	s.deps.Config.Update = u
	s.status, s.failed = "Saved to "+s.deps.ConfigPath, false
	s.refresh()
}

func (s *settingsScreen) Init() tea.Cmd { return nil }

func (s *settingsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			s.toggle(s.menu.Selected())
		}
	}
	return s, nil
}

func (s *settingsScreen) View(width, height int) string {
	c := s.deps.Config
	body := headerStyle.Render("Settings") + "\n\n"
	body += inputLabelStyle.Render("Updates") + mutedStyle.Render("  (Enter switches an option and saves it)") + "\n"
	body += s.menu.View()
	if s.status != "" {
		if s.failed {
			body += errorBoxStyle.Render(s.status) + "\n"
		} else {
			body += successStyle.Render("✓ "+s.status) + "\n"
		}
	}
	body += "\n" + inputLabelStyle.Render("Configuration") + mutedStyle.Render("  (edit "+s.deps.ConfigPath+" and restart to change these)") + "\n"
	body += fmt.Sprintf("Nginx config dir:       %s\n", c.Nginx.ConfigDir)
	body += fmt.Sprintf("sites-available:        %s\n", c.Nginx.SitesAvailable)
	body += fmt.Sprintf("sites-enabled:          %s\n", c.Nginx.SitesEnabled)
	body += fmt.Sprintf("nginx binary:           %s\n", c.Nginx.BinaryPath)
	body += fmt.Sprintf("Certificates directory: %s\n", c.Certificates.Directory)
	body += fmt.Sprintf("Nginx log directory:    %s\n", c.Logs.NginxDirectory)
	body += fmt.Sprintf("Audit log file:         %s\n", c.Logs.AuditFile)
	body += fmt.Sprintf("Backup directory:       %s (keep %d)\n", c.Backup.Directory, c.Backup.KeepCount)
	body += fmt.Sprintf("Access lists directory: %s\n", c.AccessLists.Directory)
	body += fmt.Sprintf("Access lists group:     %s\n", c.AccessLists.Group)
	body += fmt.Sprintf("Error page snippets:    %s\n", c.ErrorPages.SnippetsDirectory)
	body += fmt.Sprintf("Error pages (default):  %s\n", c.ErrorPages.PagesDirectory)
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Switch"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
