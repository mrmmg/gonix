package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/nginx"
)

type ipListsScreen struct {
	deps  Deps
	lists []iplist.List
	menu  *simpleMenu
	err   string
}

func newIPListsScreen(deps Deps) *ipListsScreen {
	s := &ipListsScreen{deps: deps}
	s.reload()
	return s
}

func (s *ipListsScreen) reload() {
	lists, err := s.deps.IPLists.Store.List()
	if err != nil {
		s.err = err.Error()
	}
	s.lists = lists
	items := []menuItem{{title: "+ Create IP Access List"}}
	for _, l := range lists {
		items = append(items, menuItem{title: l.Name, desc: ipListDesc(l)})
	}
	s.menu = newSimpleMenu(items)
}

func ipListDesc(l iplist.List) string {
	n := len(l.Entries)
	noun := "entries"
	if n == 1 {
		noun = "entry"
	}
	return fmt.Sprintf("%s, %d %s", l.Mode, n, noun)
}

func (s *ipListsScreen) Init() tea.Cmd { return nil }

func (s *ipListsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			if s.menu.Selected() == 0 {
				return s, navPush(newIPListWizard(s.deps, nil))
			}
			return s, navPush(newIPListDetailScreen(s.deps, s.lists[s.menu.Selected()-1].Name))
		}
	}
	return s, nil
}

func (s *ipListsScreen) View(width, height int) string {
	body := headerStyle.Render("IP Access Lists") + "\n"
	body += mutedStyle.Render("Allow or deny clients by IP address or range, per host or per path.") + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.ViewFit(body, height)
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

const (
	ipModeAllow = "Allowlist — only these addresses may connect"
	ipModeBlock = "Blocklist — these addresses are refused"
)

// newIPListWizard creates a list when existing is nil, otherwise edits it.
func newIPListWizard(deps Deps, existing *iplist.List) *wizardScreen {
	title := "Create IP Access List"
	modeDefault, entriesDefault := ipModeAllow, ""
	var fields []wizardField
	if existing == nil {
		fields = append(fields, wizardField{
			Key: "name", Label: "List name (letters, digits, - and _; e.g. office):", Kind: fieldText,
			Placeholder: "office",
			Validate: func(v string) error {
				if !nginx.ValidSnippetName(v) {
					return fmt.Errorf("name may only contain letters, digits, '-' and '_'")
				}
				if deps.IPLists.Store.Exists(v) {
					return fmt.Errorf("an IP access list named %q already exists", v)
				}
				return nil
			},
		})
	} else {
		title = "Edit IP Access List — " + existing.Name
		entriesDefault = strings.Join(existing.Entries, " ")
		if existing.Mode == iplist.Blocklist {
			modeDefault = ipModeBlock
		}
	}
	fields = append(fields,
		wizardField{
			Key: "mode", Label: "List type:", Kind: fieldChoice,
			Options: []string{ipModeAllow, ipModeBlock}, ChoiceDefault: modeDefault,
		},
		wizardField{
			Key: "entries", Label: "IP addresses or CIDR ranges (space or comma separated):", Kind: fieldText,
			Placeholder: "192.168.1.0/24 10.0.0.5", Default: entriesDefault,
			Validate: func(v string) error { _, err := iplist.ParseEntries(v); return err },
		},
	)

	back := func() screen {
		if existing == nil {
			return newIPListsScreen(deps)
		}
		return newIPListDetailScreen(deps, existing.Name)
	}
	return newWizard(title, fields, func(v map[string]string) (screen, tea.Cmd) {
		entries, _ := iplist.ParseEntries(v["entries"])
		l := iplist.List{Mode: iplist.Allowlist, Entries: entries}
		if v["mode"] == ipModeBlock {
			l.Mode = iplist.Blocklist
		}
		var err error
		if existing == nil {
			l.Name = v["name"]
			err = deps.IPLists.Create(backgroundCtx(), l)
		} else {
			l.Name = existing.Name
			err = deps.IPLists.Update(backgroundCtx(), l)
		}
		if err != nil {
			return newResultScreen(deps, title, false, err.Error(), back()), nil
		}
		msg := "List saved to " + deps.IPLists.Store.Path(l.Name) + "."
		if existing == nil {
			msg += " Apply it from a host's \"IP Access Rules\" menu."
		}
		msg += realIPHint(deps)
		return newResultScreen(deps, title, true, msg, newIPListDetailScreen(deps, l.Name)), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}

// realIPHint warns that IP rules match the proxy instead of the client when
// Nginx sits behind a CDN or load balancer without real IP handling.
func realIPHint(deps Deps) string {
	if _, enabled, _ := deps.RealIP.Get(); enabled {
		return ""
	}
	return "\n\nNote: if Nginx is behind Cloudflare or a load balancer, enable \"Real Client IP\" from the main menu, otherwise every request appears to come from the proxy."
}

// --- List detail ---

type ipListDetailScreen struct {
	deps Deps
	name string
	list iplist.List
	err  string
	menu *simpleMenu
}

const (
	ipdEdit = iota
	ipdView
	ipdViewHosts
	ipdDelete
)

func newIPListDetailScreen(deps Deps, name string) *ipListDetailScreen {
	s := &ipListDetailScreen{deps: deps, name: name}
	l, err := deps.IPLists.Store.Get(name)
	if err != nil {
		s.err = err.Error()
	}
	s.list = l
	s.menu = newSimpleMenu([]menuItem{
		{title: "Edit List"},
		{title: "View Snippet"},
		{title: "View Hosts Using This List"},
		{title: "Delete List"},
	})
	return s
}

func (s *ipListDetailScreen) Init() tea.Cmd { return nil }

func (s *ipListDetailScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
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

func (s *ipListDetailScreen) dispatch(choice int) (screen, tea.Cmd) {
	switch choice {
	case ipdEdit:
		l := s.list
		return newIPListWizard(s.deps, &l), nil
	case ipdView:
		data, err := os.ReadFile(s.deps.IPLists.Store.Path(s.name))
		if err != nil {
			return newResultScreen(s.deps, "View Snippet", false, err.Error(), s), nil
		}
		return newViewerScreen("IP access list: "+s.name, string(data), s), nil
	case ipdViewHosts:
		users, err := s.deps.IPLists.UsedBy(s.name)
		if err != nil {
			return newResultScreen(s.deps, "View Hosts", false, err.Error(), s), nil
		}
		content := "No hosts currently use this list."
		if len(users) > 0 {
			content = "- " + strings.Join(users, "\n- ")
		}
		return newViewerScreen("Hosts using "+s.name, content, s), nil
	case ipdDelete:
		return newConfirmScreen("Delete IP Access List",
			fmt.Sprintf("Delete IP access list %q?", s.name),
			func() (screen, tea.Cmd) {
				if err := s.deps.IPLists.Delete(backgroundCtx(), s.name); err != nil {
					return newResultScreen(s.deps, "Delete IP Access List", false, err.Error(), s), nil
				}
				return newResultScreen(s.deps, "Delete IP Access List", true, "List deleted.", newIPListsScreen(s.deps)), nil
			},
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return s, nil
}

func (s *ipListDetailScreen) View(width, height int) string {
	body := headerStyle.Render("IP Access List: "+s.name) + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err) + "\n\n"
	} else {
		body += "File: " + s.deps.IPLists.Store.Path(s.name) + "\n"
		if s.list.Mode == iplist.Blocklist {
			body += "Type: blocklist (listed addresses are refused, everyone else allowed)\n"
		} else {
			body += "Type: allowlist (only listed addresses may connect)\n"
		}
		body += "Entries:\n"
		for _, e := range s.list.Entries {
			body += "  " + e + "\n"
		}
		body += "\n"
	}
	body += s.menu.ViewFit(body, height)
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

// --- Per-host rules ---

const ipScopeHost = "Whole host"

// newHostIPRulesWizard applies an IP access list to a host, or to one of
// its reverse-proxy locations, and decides how it combines with Basic Auth.
func newHostIPRulesWizard(deps Deps, h nginx.ListedHost) screen {
	host, err := deps.Manager.GetHost(h.FileName)
	if err != nil {
		return newResultScreen(deps, "IP Access Rules", false, err.Error(), newHostDetailScreen(deps, h))
	}
	scopes := []string{ipScopeHost}
	current := map[string]string{ipScopeHost: host.IPListName}
	for _, loc := range host.Locations {
		if loc.Proxy != nil {
			scope := "Path " + loc.Path
			scopes = append(scopes, scope)
			current[scope] = loc.IPListName
		}
	}
	lists, _ := deps.IPLists.Store.List()
	options := []string{"(none)"}
	for _, l := range lists {
		options = append(options, l.Name)
	}

	fields := []wizardField{
		{Key: "scope", Label: "Apply to:", Kind: fieldChoice, Options: scopes},
	}
	// One list question per scope, so each is preselected with that scope's
	// current list.
	for _, scope := range scopes {
		scope := scope
		fields = append(fields, wizardField{
			Key: "list", Label: "IP access list for " + strings.ToLower(scope[:1]) + scope[1:] + ":",
			Kind: fieldChoice, Options: options, ChoiceDefault: current[scope],
			ShowIf: func(v map[string]string) bool { return v["scope"] == scope },
		})
	}
	fields = append(fields, wizardField{
		Key: "satisfy", Label: "This host also uses Basic Auth. Let clients in when they match the IP list OR log in (instead of requiring both)?",
		Kind: fieldBool, BoolDefault: host.SatisfyAny,
		ShowIf: func(v map[string]string) bool {
			return v["scope"] == ipScopeHost && v["list"] != "(none)" && host.AccessListName != ""
		},
	})

	back := func() screen { return newHostDetailScreen(deps, h) }
	return newWizard("IP Access Rules — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "IP Access Rules", false, err.Error(), back()), nil
		}
		list := v["list"]
		if list == "(none)" {
			list = ""
		}
		if v["scope"] == ipScopeHost {
			host.IPListName = list
			host.SatisfyAny = list != "" && host.AccessListName != "" && v["satisfy"] == "yes"
		} else {
			path := strings.TrimPrefix(v["scope"], "Path ")
			for i := range host.Locations {
				if host.Locations[i].Path == path {
					host.Locations[i].IPListName = list
				}
			}
		}
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "IP Access Rules", false, err.Error(), back()), nil
		}
		return newResultScreen(deps, "IP Access Rules", true,
			"IP access rules updated for "+h.ServerName+"."+realIPHint(deps), back()), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}
