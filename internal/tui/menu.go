package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/lab"
	"github.com/mrmmg/gonix/internal/update"
)

// mainMenu is the application's entry screen.
type mainMenu struct {
	deps    Deps
	menu    *simpleMenu
	actions []int           // mi* action of each row, in display order
	alerts  []string        // certificates that are expired or about to be
	update  *update.Release // a newer GoNix release, when one is known
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
	miUpdates
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
		{title: "Updates", desc: "check for a new GoNix release and how to install it"},
		{title: "Settings", desc: "GoNix configuration and update options"},
		{title: "Lab", desc: "a separate Nginx with hand-written test hosts, for trying GoNix"},
		{title: "Exit", desc: ""},
	}
	m := &mainMenu{deps: deps, alerts: certAlerts(deps)}
	if !inLab && deps.Config.Update.Check {
		if rel, ok := knownRelease(deps); ok {
			m.update = &rel
		}
	}
	if m.update != nil {
		all[miUpdates].desc = infoStyle.Render("version " + m.update.Version + " is available")
	}
	if len(m.alerts) > 0 {
		all[miCertificates].desc = dangerStyle.Render("⚠ expiring or expired: needs attention")
	}
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

// Init looks for a newer release in the background (at most once a day), so
// opening GoNix is never slowed down by it.
func (s *mainMenu) Init() tea.Cmd {
	if inLab || !s.deps.Config.Update.Check {
		return nil
	}
	if _, fresh, ok := update.ReadCache(update.CachePath(s.deps.Config.Logs.AuditFile)); ok && fresh {
		return nil
	}
	return checkUpdateCmd(s.deps, false)
}

func (s *mainMenu) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case updateCheckedMsg:
		if msg.err == nil && update.Newer(msg.rel.Tag, Version) && s.update == nil {
			s.update = &msg.rel
			for i, a := range s.actions {
				if a == miUpdates {
					items := s.menu.items
					items[i].desc = infoStyle.Render("version " + msg.rel.Version + " is available")
					s.menu.SetItems(items)
				}
			}
		}
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
			case miUpdates:
				return s, navPush(newUpdateScreen(s.deps))
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
	if len(s.alerts) > 0 {
		body += callout(width-4, calloutDanger, "Certificates need attention", s.alerts) + "\n"
	}
	if s.update != nil {
		body += callout(width-4, calloutInfo, "GoNix "+s.update.Version+" is available (you have "+Version+")", []string{
			"Update from a terminal:  sudo gonix update",
			"Release notes and how it works: Updates menu",
			"",
			"Note: this check can be turned off in Settings (\"Check for updates\").",
		}) + "\n"
	}
	body += s.menu.ViewFit(body, height)
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"q", "Quit"}}
	return renderFrame(width, height, "GONIX", body, help)
}

// certAlerts lists, one line each and with the names lined up, the
// certificates (GoNix's and Let's Encrypt's) that are expired or have 14
// days or less left, so they are noticed without opening the Certificates
// screen. A long list is cut, with a pointer to the Certificates screen.
func certAlerts(deps Deps) []string {
	const most = 5
	var list []certificates.Certificate
	for _, c := range allCerts(deps) {
		if c.NeedsAttention() {
			list = append(list, c)
		}
	}
	w := certNameWidth(list)
	var out []string
	for i, c := range list {
		if i == most {
			out = append(out, fmt.Sprintf("… and %d more — open Certificates to see them all", len(list)-most))
			break
		}
		out = append(out, fmt.Sprintf("%-*s  %-15s  %s", w, c.Domain, "["+c.SourceLabel()+"]", c.Short()))
	}
	return out
}

func certNameWidth(certs []certificates.Certificate) int {
	w := 0
	for _, c := range certs {
		w = max(w, len([]rune(c.Domain)))
	}
	return w
}
