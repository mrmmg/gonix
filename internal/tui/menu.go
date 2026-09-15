package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// mainMenu is the application's entry screen.
type mainMenu struct {
	deps Deps
	menu *simpleMenu
}

const (
	miAddHost = iota
	miManageHosts
	miCertificates
	miNginxStatus
	miAccessLists
	miSettings
	miExit
)

func newMainMenu(deps Deps) *mainMenu {
	return &mainMenu{
		deps: deps,
		menu: newSimpleMenu([]menuItem{
			{title: "Add New Host", desc: "create a reverse proxy or static host"},
			{title: "Manage Hosts", desc: "enable/disable, edit, delete existing hosts"},
			{title: "Certificates", desc: "inspect TLS certificates"},
			{title: "Nginx Status", desc: "service control & process monitoring"},
			{title: "Access Lists", desc: "HTTP Basic Authentication"},
			{title: "Settings", desc: "GoNix configuration"},
			{title: "Exit", desc: ""},
		}),
	}
}

func (s *mainMenu) Init() tea.Cmd { return nil }

func (s *mainMenu) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.menu.HandleKey(msg) {
			return s, nil
		}
		switch msg.String() {
		case "enter":
			switch s.menu.Selected() {
			case miAddHost:
				return s, navPush(newAddHostWizard(s.deps))
			case miManageHosts:
				return s, navPush(newHostsListScreen(s.deps))
			case miCertificates:
				return s, navPush(newCertificatesScreen(s.deps))
			case miNginxStatus:
				return s, navPush(newStatusScreen(s.deps))
			case miAccessLists:
				return s, navPush(newAccessListsScreen(s.deps))
			case miSettings:
				return s, navPush(newSettingsScreen(s.deps))
			case miExit:
				return s, tea.Quit
			}
		case "q", "esc":
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *mainMenu) View(width, height int) string {
	body := headerStyle.Render("Main Menu") + "\n" + s.menu.View()
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"q", "Quit"}}
	return renderFrame(width, height, "GONIX", body, help)
}
