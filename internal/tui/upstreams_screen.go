package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/upstreams"
)

type upstreamsScreen struct {
	deps   Deps
	groups []upstreams.Group
	menu   *simpleMenu
	err    string
}

func newUpstreamsScreen(deps Deps) *upstreamsScreen {
	s := &upstreamsScreen{deps: deps}
	groups, err := deps.Upstreams.List()
	if err != nil {
		s.err = err.Error()
	}
	s.groups = groups
	items := []menuItem{{title: "+ Create Upstream Group"}}
	for _, g := range groups {
		items = append(items, menuItem{title: g.Name, desc: fmt.Sprintf("%s, %d servers", g.Method, len(g.Servers))})
	}
	s.menu = newSimpleMenu(items)
	return s
}

func (s *upstreamsScreen) Init() tea.Cmd { return nil }

func (s *upstreamsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			if s.menu.Selected() == 0 {
				return s, navPush(newUpstreamWizard(s.deps, nil))
			}
			return s, navPush(newUpstreamDetailScreen(s.deps, s.groups[s.menu.Selected()-1].Name))
		}
	}
	return s, nil
}

func (s *upstreamsScreen) View(width, height int) string {
	body := headerStyle.Render("Upstream Groups") + "\n"
	body += mutedStyle.Render("Several backends behind one name, with load balancing and failover.") + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.View()
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

var methodLabels = map[string]string{
	upstreams.RoundRobin: "round_robin — take turns",
	upstreams.LeastConn:  "least_conn — server with the fewest active connections",
	upstreams.IPHash:     "ip_hash — each client sticks to one server",
}

// newUpstreamWizard creates a group when existing is nil, otherwise edits it.
func newUpstreamWizard(deps Deps, existing *upstreams.Group) *wizardScreen {
	title := "Create Upstream Group"
	g := upstreams.Group{Method: upstreams.RoundRobin, MaxFails: 3, FailTimeout: "10s"}
	var fields []wizardField
	if existing == nil {
		fields = append(fields, wizardField{
			Key: "name", Label: "Group name (letters, digits, - and _; e.g. app):", Kind: fieldText, Placeholder: "app",
			Validate: func(v string) error {
				if !nginx.ValidSnippetName(v) {
					return fmt.Errorf("name may only contain letters, digits, '-' and '_'")
				}
				if deps.Upstreams.Exists(v) {
					return fmt.Errorf("an upstream group named %q already exists", v)
				}
				return nil
			},
		})
	} else {
		title = "Edit Upstream Group — " + existing.Name
		g = *existing
	}
	methods := make([]string, len(upstreams.Methods))
	for i, m := range upstreams.Methods {
		methods[i] = methodLabels[m]
	}
	fields = append(fields,
		wizardField{Key: "method", Label: "Load balancing:", Kind: fieldChoice, Options: methods, ChoiceDefault: methodLabels[g.Method]},
		wizardField{Key: "servers", Label: "Servers, comma separated (host:port, optional weight=N or backup):", Kind: fieldText,
			Placeholder: "10.0.0.11:3000, 10.0.0.12:3000 weight=2, 10.0.0.13:3000 backup",
			Default:     upstreams.FormatServers(g.Servers),
			Validate:    func(v string) error { _, err := upstreams.ParseServers(v); return err }},
		wizardField{Key: "max_fails", Label: "Failed attempts before a server is skipped (0 = never skip):", Kind: fieldText,
			Default: strconv.Itoa(g.MaxFails), Validate: intRange(0, 100)},
		wizardField{Key: "fail_timeout", Label: "How long a failed server is skipped:", Kind: fieldText,
			Default: g.FailTimeout, Placeholder: "10s"},
	)

	back := func() screen {
		if existing == nil {
			return newUpstreamsScreen(deps)
		}
		return newUpstreamDetailScreen(deps, existing.Name)
	}
	return newWizard(title, fields, func(v map[string]string) (screen, tea.Cmd) {
		out := upstreams.Group{Method: upstreams.RoundRobin, FailTimeout: v["fail_timeout"]}
		for m, label := range methodLabels {
			if label == v["method"] {
				out.Method = m
			}
		}
		out.Servers, _ = upstreams.ParseServers(v["servers"])
		out.MaxFails, _ = strconv.Atoi(v["max_fails"])
		var err error
		if existing == nil {
			out.Name = v["name"]
			err = deps.Upstreams.Create(backgroundCtx(), out)
		} else {
			out.Name = existing.Name
			err = deps.Upstreams.Update(backgroundCtx(), out)
		}
		if err != nil {
			return newResultScreen(deps, title, false, err.Error()+includeWarning(deps), back()), nil
		}
		msg := "Upstream group " + out.Name + " saved."
		if existing == nil {
			msg += " Choose it as the upstream when adding a host or path, or in Proxy Configuration."
		}
		return newResultScreen(deps, title, true, msg+includeWarning(deps), newUpstreamDetailScreen(deps, out.Name)), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}

type upstreamDetailScreen struct {
	deps  Deps
	name  string
	group upstreams.Group
	err   string
	menu  *simpleMenu
}

func newUpstreamDetailScreen(deps Deps, name string) *upstreamDetailScreen {
	s := &upstreamDetailScreen{deps: deps, name: name}
	g, err := deps.Upstreams.Get(name)
	if err != nil {
		s.err = err.Error()
	}
	s.group = g
	s.menu = newSimpleMenu([]menuItem{
		{title: "Edit Group"},
		{title: "View Configuration"},
		{title: "View Hosts Using This Group"},
		{title: "Delete Group"},
	})
	return s
}

func (s *upstreamDetailScreen) Init() tea.Cmd { return nil }

func (s *upstreamDetailScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
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

func (s *upstreamDetailScreen) dispatch(choice int) (screen, tea.Cmd) {
	switch choice {
	case 0:
		g := s.group
		return newUpstreamWizard(s.deps, &g), nil
	case 1:
		data, err := os.ReadFile(s.deps.Upstreams.Path(s.name))
		if err != nil {
			return newResultScreen(s.deps, "View Configuration", false, err.Error(), s), nil
		}
		return newViewerScreen("Upstream group: "+s.name, string(data), s), nil
	case 2:
		users, err := s.deps.Upstreams.UsedBy(s.name)
		if err != nil {
			return newResultScreen(s.deps, "View Hosts", false, err.Error(), s), nil
		}
		content := "No hosts currently use this group."
		if len(users) > 0 {
			content = "- " + strings.Join(users, "\n- ")
		}
		return newViewerScreen("Hosts using "+s.name, content, s), nil
	case 3:
		return newConfirmScreen("Delete Upstream Group", fmt.Sprintf("Delete upstream group %q?", s.name),
			func() (screen, tea.Cmd) {
				if err := s.deps.Upstreams.Delete(backgroundCtx(), s.name); err != nil {
					return newResultScreen(s.deps, "Delete Upstream Group", false, err.Error(), s), nil
				}
				return newResultScreen(s.deps, "Delete Upstream Group", true, "Group deleted.", newUpstreamsScreen(s.deps)), nil
			},
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return s, nil
}

func (s *upstreamDetailScreen) View(width, height int) string {
	body := headerStyle.Render("Upstream Group: "+s.name) + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err) + "\n\n"
	} else {
		body += "Load balancing: " + s.group.Method + "\n"
		body += fmt.Sprintf("Failover: skip a server for %s after %d failed attempts\n", s.group.FailTimeout, s.group.MaxFails)
		body += "Servers:\n"
		for _, sv := range s.group.Servers {
			line := "  " + sv.Address
			if sv.Weight > 1 {
				line += fmt.Sprintf("  weight %d", sv.Weight)
			}
			if sv.Backup {
				line += "  (backup)"
			}
			body += line + "\n"
		}
		body += "\n"
	}
	body += s.menu.View()
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

// --- Upstream target choice, shared by the proxy wizards ---

const upstreamSingle = "Single server (host and port)"

func upstreamTargetOptions(deps Deps) []string {
	groups, _ := deps.Upstreams.List()
	if len(groups) == 0 {
		return nil
	}
	options := []string{upstreamSingle}
	for _, g := range groups {
		options = append(options, "Group: "+g.Name)
	}
	return options
}

// upstreamTargetField asks whether to proxy to a single server or a group.
// It is only shown when groups exist; showIf may be nil.
func upstreamTargetField(deps Deps, current string, showIf func(map[string]string) bool) []wizardField {
	options := upstreamTargetOptions(deps)
	if options == nil {
		return nil
	}
	def := upstreamSingle
	if current != "" {
		def = "Group: " + current
	}
	return []wizardField{{Key: "upstream_target", Label: "Proxy to:", Kind: fieldChoice, Options: options, ChoiceDefault: def, ShowIf: showIf}}
}

// upstreamGroupChoice returns the chosen group, or "" for a single server.
func upstreamGroupChoice(v map[string]string) string {
	if isSingleUpstream(v) {
		return ""
	}
	return strings.TrimPrefix(v["upstream_target"], "Group: ")
}

func isSingleUpstream(v map[string]string) bool {
	return v["upstream_target"] == "" || v["upstream_target"] == upstreamSingle
}
