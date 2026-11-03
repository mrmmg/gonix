package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/certificates"
)

// mainMenu is the application's entry screen.
type mainMenu struct {
	deps    Deps
	menu    *simpleMenu
	warning string // e.g. certificates about to expire
}

const (
	miAddHost = iota
	miManageHosts
	miCertificates
	miNginxStatus
	miAccessLists
	miIPLists
	miRealIP
	miRateLimits
	miUpstreams
	miErrorPages
	miConfigCheck
	miSettings
	miExit
)

func newMainMenu(deps Deps) *mainMenu {
	return &mainMenu{
		deps:    deps,
		warning: certificateWarning(deps),
		menu: newSimpleMenu([]menuItem{
			{title: "Add New Host", desc: "create a reverse proxy or static host"},
			{title: "Manage Hosts", desc: "enable/disable, edit, delete existing hosts"},
			{title: "Certificates", desc: "inspect TLS certificates"},
			{title: "Nginx Status", desc: "service control & process monitoring"},
			{title: "Access Lists", desc: "HTTP Basic Authentication"},
			{title: "IP Access Lists", desc: "allow or deny clients by IP address"},
			{title: "Real Client IP", desc: "Cloudflare / load balancer client addresses"},
			{title: "Rate Limits", desc: "limit requests and connections per client"},
			{title: "Upstream Groups", desc: "load balancing across several backends"},
			{title: "Error Pages", desc: "custom error page snippets for hosts"},
			{title: "Config Check", desc: "conflicts, missing files, expiring certificates, dead upstreams"},
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
			case miIPLists:
				return s, navPush(newIPListsScreen(s.deps))
			case miRealIP:
				return s, navPush(newRealIPScreen(s.deps))
			case miRateLimits:
				return s, navPush(newRateLimitsScreen(s.deps))
			case miUpstreams:
				return s, navPush(newUpstreamsScreen(s.deps))
			case miErrorPages:
				return s, navPush(newErrorPagesScreen(s.deps))
			case miConfigCheck:
				return s, navPush(newConfigCheckScreen(s.deps))
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
	if s.warning != "" {
		body += "\n" + dangerStyle.Render("⚠ "+s.warning)
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"q", "Quit"}}
	return renderFrame(width, height, "GONIX", body, help)
}

// certificateWarning summarizes certificates that are expired or due for
// renewal, so they are noticed without opening the Certificates screen.
func certificateWarning(deps Deps) string {
	certs, err := certificates.Scan(deps.Config.Certificates.Directory)
	if err != nil {
		return ""
	}
	var expired, due []string
	for _, c := range certs {
		switch {
		case c.Status == certificates.StatusExpired:
			expired = append(expired, c.Domain)
		case c.DueForRenewal():
			due = append(due, c.Domain)
		}
	}
	var parts []string
	if len(expired) > 0 {
		parts = append(parts, "expired certificates: "+strings.Join(expired, ", "))
	}
	if len(due) > 0 {
		parts = append(parts, "due for renewal: "+strings.Join(due, ", "))
	}
	return strings.Join(parts, "; ")
}
