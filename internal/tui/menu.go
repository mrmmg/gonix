package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/lab"
)

// mainMenu is the application's entry screen.
type mainMenu struct {
	deps    Deps
	menu    *simpleMenu
	actions []int  // mi* action of each row, in display order
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
	miGeo
	miRateLimits
	miUpstreams
	miStreams
	miErrorPages
	miConfigCheck
	miHistory
	miMonitoring
	miSettings
	miLab
	miExit
)

func newMainMenu(deps Deps) *mainMenu {
	all := []menuItem{
		{title: "Add New Host", desc: "create a reverse proxy or static host"},
		{title: "Manage Hosts", desc: "enable/disable, edit, delete existing hosts"},
		{title: "Certificates", desc: "inspect TLS certificates"},
		{title: "Nginx Status", desc: "service control & process monitoring"},
		{title: "Access Lists", desc: "HTTP Basic Authentication"},
		{title: "IP Access Lists", desc: "allow or deny clients by IP address"},
		{title: "Real Client IP", desc: "Cloudflare / load balancer client addresses"},
		{title: "Country Blocking", desc: "allow or refuse clients by country (GeoIP)"},
		{title: "Rate Limits", desc: "limit requests and connections per client"},
		{title: "Upstream Groups", desc: "load balancing across several backends"},
		{title: "Stream Proxies", desc: "forward TCP/UDP ports (databases, SSH, DNS, ...)"},
		{title: "Error Pages", desc: "custom error page snippets for hosts"},
		{title: "Config Check", desc: "conflicts, missing files, expiring certificates, dead upstreams"},
		{title: "Configuration History", desc: "every change as a Git commit; diff and restore"},
		{title: "Monitoring", desc: "Prometheus exporter for Grafana: control and settings"},
		{title: "Settings", desc: "GoNix configuration"},
		{title: "Lab", desc: "a separate Nginx with hand-written test hosts, for trying GoNix"},
		{title: "Exit", desc: ""},
	}
	m := &mainMenu{deps: deps, warning: certificateWarning(deps)}
	var items []menuItem
	for action, item := range all {
		switch {
		case action == miLab && (inLab || !lab.Enabled()):
			continue
		case action == miExit && inLab:
			item = menuItem{title: "Exit Lab", desc: "back to the real server"}
		}
		items = append(items, item)
		m.actions = append(m.actions, action)
	}
	m.menu = newSimpleMenu(items)
	return m
}

// leave quits the TUI, or, inside the lab, goes back to the real server.
func (s *mainMenu) leave() tea.Cmd {
	if inLab {
		return func() tea.Msg { return labExitMsg{} }
	}
	return tea.Quit
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
			action := -1
			if i := s.menu.Selected(); i < len(s.actions) {
				action = s.actions[i]
			}
			switch action {
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
			case miGeo:
				return s, navPush(newGeoScreen(s.deps))
			case miRateLimits:
				return s, navPush(newRateLimitsScreen(s.deps))
			case miUpstreams:
				return s, navPush(newUpstreamsScreen(s.deps))
			case miStreams:
				return s, navPush(newStreamsScreen(s.deps))
			case miErrorPages:
				return s, navPush(newErrorPagesScreen(s.deps))
			case miConfigCheck:
				return s, navPush(newConfigCheckScreen(s.deps))
			case miHistory:
				return s, navPush(newHistoryScreen(s.deps))
			case miMonitoring:
				return s, navPush(newMonitoringScreen(s.deps))
			case miSettings:
				return s, navPush(newSettingsScreen(s.deps))
			case miLab:
				return s, navPush(newLabScreen(s.deps))
			case miExit:
				return s, s.leave()
			}
		case "q", "esc":
			return s, s.leave()
		}
	}
	return s, nil
}

func (s *mainMenu) View(width, height int) string {
	body := headerStyle.Render("Main Menu") + "\n"
	if inLab {
		body += dangerStyle.Render("LAB: changes go to the lab's Nginx ("+s.deps.Config.Nginx.ConfigDir+"), not the real server") + "\n"
	}
	if s.warning != "" {
		body += dangerStyle.Render("⚠ "+s.warning) + "\n"
	}
	body += s.menu.ViewFit(body, height)
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
