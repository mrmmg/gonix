package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
)

// hostsListScreen implements "Manage Hosts": a searchable list of every
// discovered host (managed or not), showing enabled state, protocol and any
// filesystem-level anomalies (broken symlinks).
type hostsListScreen struct {
	deps    Deps
	hosts   []nginx.ListedHost
	menu    *simpleMenu
	loadErr string
}

func newHostsListScreen(deps Deps) *hostsListScreen {
	s := &hostsListScreen{deps: deps, menu: newSimpleMenu(nil)}
	s.reload()
	return s
}

func (s *hostsListScreen) reload() {
	hostList, err := s.deps.Manager.ListHosts()
	if err != nil {
		s.loadErr = err.Error()
		return
	}
	s.hosts = hostList
	items := make([]menuItem, 0, len(hostList)+1)
	for _, h := range hostList {
		dot := disabledDot
		state := "DISABLED"
		if h.Enabled {
			dot = enabledDot
			state = "ENABLED"
		}
		proto := "HTTP"
		if h.SSL.Enabled {
			proto = "HTTPS"
		}
		managed := ""
		if !h.Managed {
			managed = mutedStyle.Render(" [unmanaged]")
		}
		if h.BrokenSymlink {
			managed += dangerStyle.Render(" [broken symlink]")
		}
		items = append(items, menuItem{
			title: fmt.Sprintf("%s %-30s", dot, h.ServerName),
			desc:  fmt.Sprintf("%-6s %s%s", proto, state, managed),
		})
	}
	if len(items) == 0 {
		items = append(items, menuItem{title: "(no hosts found)"})
	}
	s.menu.SetItems(items)
}

func (s *hostsListScreen) Init() tea.Cmd { return nil }

func (s *hostsListScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.menu.HandleKey(msg) {
			return s, nil
		}
		switch msg.String() {
		case "enter":
			if len(s.hosts) == 0 {
				return s, nil
			}
			h := s.hosts[s.menu.Selected()]
			return s, navPush(newHostDetailScreen(s.deps, h))
		case "esc", "q":
			return s, navPop()
		case "r":
			s.reload()
			return s, nil
		}
	}
	return s, nil
}

func (s *hostsListScreen) View(width, height int) string {
	body := headerStyle.Render("Hosts") + "\n"
	if s.loadErr != "" {
		body += errorBoxStyle.Render(s.loadErr)
	} else {
		body += s.menu.View()
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Manage"}, {"r", "Refresh"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
