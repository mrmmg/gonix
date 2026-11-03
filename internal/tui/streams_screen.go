package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/streams"
	"github.com/mrmmg/gonix/internal/upstreams"
)

// streamsScreen lists TCP/UDP stream proxies and offers to set up stream
// support when it is not active yet.
type streamsScreen struct {
	deps    Deps
	proxies []streams.Proxy
	ready   bool
	why     string
	setup   bool // first menu row is "Set up stream support"
	menu    *simpleMenu
	err     string
}

func newStreamsScreen(deps Deps) *streamsScreen {
	s := &streamsScreen{deps: deps}
	s.ready, s.why = deps.Streams.Ready()
	proxies, err := deps.Streams.List()
	if err != nil {
		s.err = err.Error()
	}
	s.proxies = proxies
	var items []menuItem
	if !s.ready && deps.Streams.CanSetUp() {
		s.setup = true
		items = append(items, menuItem{title: "Set Up Stream Support", desc: "create the stream { } block in " + deps.Streams.WrapperPath()})
	}
	items = append(items, menuItem{title: "+ Create Stream Proxy"})
	for _, p := range proxies {
		dot := enabledDot
		if !p.Enabled {
			dot = disabledDot
		}
		items = append(items, menuItem{title: dot + " " + p.Name, desc: fmt.Sprintf("%s %d → %s", p.Protocol(), p.Port, p.Upstream)})
	}
	s.menu = newSimpleMenu(items)
	return s
}

func (s *streamsScreen) Init() tea.Cmd { return nil }

func (s *streamsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			i := s.menu.Selected()
			if s.setup {
				if i == 0 {
					if err := s.deps.Streams.SetUp(backgroundCtx()); err != nil {
						return newResultScreen(s.deps, "Stream Support", false, err.Error(), newStreamsScreen(s.deps)), nil
					}
					return newResultScreen(s.deps, "Stream Support", true, "Stream support is set up.", newStreamsScreen(s.deps)), nil
				}
				i--
			}
			if i == 0 {
				return s, navPush(newStreamWizard(s.deps, nil))
			}
			return s, navPush(newStreamDetailScreen(s.deps, s.proxies[i-1].Name))
		}
	}
	return s, nil
}

func (s *streamsScreen) View(width, height int) string {
	body := headerStyle.Render("Stream Proxies (TCP/UDP)") + "\n"
	body += mutedStyle.Render("Forward raw TCP or UDP ports, e.g. databases, SSH, DNS or game servers.") + "\n\n"
	if !s.ready {
		body += errorBoxStyle.Render(s.why) + "\n\n"
	}
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.View()
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

func newStreamWizard(deps Deps, existing *streams.Proxy) *wizardScreen {
	title := "Create Stream Proxy"
	p := streams.Proxy{ConnectTimeout: "10s", Timeout: "10m", Enabled: true}
	var fields []wizardField
	if existing == nil {
		fields = append(fields, wizardField{
			Key: "name", Label: "Name (letters, digits, - and _; e.g. postgres):", Kind: fieldText, Placeholder: "postgres",
			Validate: func(v string) error {
				if !nginx.ValidSnippetName(v) {
					return fmt.Errorf("name may only contain letters, digits, '-' and '_'")
				}
				if deps.Streams.Exists(v) {
					return fmt.Errorf("a stream proxy named %q already exists", v)
				}
				return nil
			},
		})
	} else {
		title = "Edit Stream Proxy — " + existing.Name
		p = *existing
	}
	port := ""
	if p.Port > 0 {
		port = strconv.Itoa(p.Port)
	}
	lists, _ := deps.IPLists.Store.List()
	ipOptions := []string{"(none)"}
	for _, l := range lists {
		ipOptions = append(ipOptions, l.Name)
	}
	fields = append(fields,
		wizardField{Key: "protocol", Label: "Protocol:", Kind: fieldChoice, Options: []string{"tcp", "udp"}, ChoiceDefault: p.Protocol()},
		wizardField{Key: "port", Label: "Port Nginx listens on:", Kind: fieldText, Placeholder: "5432", Default: port, Validate: validatePort},
		wizardField{Key: "upstream", Label: "Backend (host:port):", Kind: fieldText, Placeholder: "10.0.0.20:5432", Default: p.Upstream,
			Validate: upstreams.ValidateAddress},
		wizardField{Key: "connect_timeout", Label: "Connect timeout:", Kind: fieldText, Default: p.ConnectTimeout},
		wizardField{Key: "timeout", Label: "Idle timeout (connection closed after no traffic for this long):", Kind: fieldText, Default: p.Timeout},
		wizardField{Key: "ip_list", Label: "IP access list:", Kind: fieldChoice, Options: ipOptions, ChoiceDefault: p.IPListName},
	)
	back := func() screen {
		if existing == nil {
			return newStreamsScreen(deps)
		}
		return newStreamDetailScreen(deps, existing.Name)
	}
	return newWizard(title, fields, func(v map[string]string) (screen, tea.Cmd) {
		out := streams.Proxy{
			UDP: v["protocol"] == "udp", Upstream: v["upstream"],
			ConnectTimeout: v["connect_timeout"], Timeout: v["timeout"], Enabled: p.Enabled,
		}
		out.Port, _ = strconv.Atoi(v["port"])
		if v["ip_list"] != "(none)" {
			out.IPListName = v["ip_list"]
		}
		if existing == nil {
			out.Name = v["name"]
		} else {
			out.Name = existing.Name
		}
		if err := deps.Streams.Save(backgroundCtx(), out, existing == nil); err != nil {
			return newResultScreen(deps, title, false, err.Error(), back()), nil
		}
		msg := fmt.Sprintf("Nginx now forwards %s port %d to %s.", out.Protocol(), out.Port, out.Upstream)
		if ready, why := deps.Streams.Ready(); !ready {
			msg += "\n\nIt takes effect once stream support is set up:\n" + why
		}
		return newResultScreen(deps, title, true, msg, newStreamDetailScreen(deps, out.Name)), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}

type streamDetailScreen struct {
	deps  Deps
	name  string
	proxy streams.Proxy
	err   string
	menu  *simpleMenu
}

func newStreamDetailScreen(deps Deps, name string) *streamDetailScreen {
	s := &streamDetailScreen{deps: deps, name: name}
	p, err := deps.Streams.Get(name)
	if err != nil {
		s.err = err.Error()
	}
	s.proxy = p
	toggle := "Disable"
	if !p.Enabled {
		toggle = "Enable"
	}
	s.menu = newSimpleMenu([]menuItem{{title: toggle}, {title: "Edit"}, {title: "View Configuration"}, {title: "Delete"}})
	return s
}

func (s *streamDetailScreen) Init() tea.Cmd { return nil }

func (s *streamDetailScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			return s.dispatch(s.menu.Selected())
		}
	}
	return s, nil
}

func (s *streamDetailScreen) dispatch(choice int) (screen, tea.Cmd) {
	switch choice {
	case 0:
		if err := s.deps.Streams.SetEnabled(backgroundCtx(), s.name, !s.proxy.Enabled); err != nil {
			return newResultScreen(s.deps, "Stream Proxy", false, err.Error(), s), nil
		}
		return newStreamDetailScreen(s.deps, s.name), nil
	case 1:
		p := s.proxy
		return newStreamWizard(s.deps, &p), nil
	case 2:
		content, err := streams.Render(s.proxy, s.deps.Config.ErrorPages.SnippetsDirectory)
		if err != nil {
			return newResultScreen(s.deps, "View Configuration", false, err.Error(), s), nil
		}
		return newViewerScreen("Stream proxy: "+s.name, content, s), nil
	case 3:
		return newConfirmScreen("Delete Stream Proxy", fmt.Sprintf("Delete stream proxy %q?", s.name),
			func() (screen, tea.Cmd) {
				if err := s.deps.Streams.Delete(backgroundCtx(), s.name); err != nil {
					return newResultScreen(s.deps, "Delete Stream Proxy", false, err.Error(), s), nil
				}
				return newResultScreen(s.deps, "Delete Stream Proxy", true, "Stream proxy deleted.", newStreamsScreen(s.deps)), nil
			},
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return s, nil
}

func (s *streamDetailScreen) View(width, height int) string {
	body := headerStyle.Render("Stream Proxy: "+s.name) + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err) + "\n\n"
	} else {
		state := enabledDot + " ENABLED"
		if !s.proxy.Enabled {
			state = disabledDot + " DISABLED"
		}
		body += state + "\n"
		body += fmt.Sprintf("Listen:  %s %d\nBackend: %s\n", s.proxy.Protocol(), s.proxy.Port, s.proxy.Upstream)
		if s.proxy.IPListName != "" {
			body += "IP access list: " + s.proxy.IPListName + "\n"
		}
		body += "\n"
	}
	body += s.menu.View()
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
