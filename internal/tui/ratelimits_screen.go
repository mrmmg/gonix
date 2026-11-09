package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/ratelimit"
)

type rateLimitsScreen struct {
	deps   Deps
	limits []ratelimit.Limit
	menu   *simpleMenu
	err    string
}

func newRateLimitsScreen(deps Deps) *rateLimitsScreen {
	s := &rateLimitsScreen{deps: deps}
	limits, err := deps.RateLimits.List()
	if err != nil {
		s.err = err.Error()
	}
	s.limits = limits
	items := []menuItem{{title: "+ Create Rate Limit", pinned: true}}
	for _, l := range limits {
		items = append(items, menuItem{title: l.Name, desc: l.Describe()})
	}
	s.menu = newSearchMenu(items, "name or limit")
	return s
}

func (s *rateLimitsScreen) Init() tea.Cmd { return nil }

func (s *rateLimitsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			if s.menu.Selected() < 0 {
				return s, nil
			}
			if s.menu.Selected() == 0 {
				return s, navPush(newRateLimitWizard(s.deps, nil))
			}
			return s, navPush(newRateLimitDetailScreen(s.deps, s.limits[s.menu.Selected()-1].Name))
		}
	}
	return s, nil
}

func (s *rateLimitsScreen) View(width, height int) string {
	body := headerStyle.Render("Rate Limits") + "\n"
	body += mutedStyle.Render("Limit requests and connections per client IP; clients over the limit get 429.") + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.ViewFit(body, height)
	}
	help := searchHelp(s.menu, [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}})
	return renderFrame(width, height, "GONIX", body, help)
}

const rateLimitCustom = "Custom"

func rateLimitPresetLabel(l ratelimit.Limit) string { return l.Name + " — " + l.Describe() }

// newRateLimitWizard creates a limit when existing is nil (starting from a
// preset or custom values), otherwise edits it.
func newRateLimitWizard(deps Deps, existing *ratelimit.Limit) *wizardScreen {
	title := "Create Rate Limit"
	var fields []wizardField
	start := ratelimit.Limit{Rate: 10, Per: "s", Burst: 20, NoDelay: true}
	isCustom := func(v map[string]string) bool { return existing != nil || v["preset"] == rateLimitCustom }

	if existing == nil {
		options := []string{}
		for _, p := range ratelimit.Presets {
			options = append(options, rateLimitPresetLabel(p))
		}
		options = append(options, rateLimitCustom)
		fields = append(fields,
			wizardField{Key: "preset", Label: "Start from:", Kind: fieldChoice, Options: options},
			wizardField{
				Key: "name", Label: "Rate limit name (letters, digits, - and _):", Kind: fieldText,
				Placeholder: "api",
				Validate: func(v string) error {
					if !nginx.ValidSnippetName(v) {
						return fmt.Errorf("name may only contain letters, digits, '-' and '_'")
					}
					if deps.RateLimits.Exists(v) {
						return fmt.Errorf("a rate limit named %q already exists", v)
					}
					return nil
				},
			},
		)
	} else {
		title = "Edit Rate Limit — " + existing.Name
		start = *existing
	}
	per := "per second"
	if start.Per == "m" {
		per = "per minute"
	}
	fields = append(fields,
		wizardField{Key: "rate", Label: "Requests allowed per client:", Kind: fieldText,
			Default: strconv.Itoa(start.Rate), Validate: intRange(1, 100000), ShowIf: isCustom},
		wizardField{Key: "per", Label: "Per:", Kind: fieldChoice, Options: []string{"per second", "per minute"},
			ChoiceDefault: per, ShowIf: isCustom},
		wizardField{Key: "burst", Label: "Burst (extra requests tolerated in a spike, 0 for none):", Kind: fieldText,
			Default: strconv.Itoa(start.Burst), Validate: intRange(0, 100000), ShowIf: isCustom},
		wizardField{Key: "nodelay", Label: "Serve burst requests immediately instead of slowing them down?", Kind: fieldBool,
			BoolDefault: start.NoDelay, ShowIf: isCustom},
		wizardField{Key: "conn", Label: "Max concurrent connections per client (0 for unlimited):", Kind: fieldText,
			Default: strconv.Itoa(start.Conn), Validate: intRange(0, 100000), ShowIf: isCustom},
	)

	back := func() screen {
		if existing == nil {
			return newRateLimitsScreen(deps)
		}
		return newRateLimitDetailScreen(deps, existing.Name)
	}
	return newWizard(title, fields, func(v map[string]string) (screen, tea.Cmd) {
		var l ratelimit.Limit
		if !isCustom(v) {
			for _, p := range ratelimit.Presets {
				if rateLimitPresetLabel(p) == v["preset"] {
					l = p
				}
			}
		} else {
			l.Rate, _ = strconv.Atoi(v["rate"])
			l.Per = "s"
			if v["per"] == "per minute" {
				l.Per = "m"
			}
			l.Burst, _ = strconv.Atoi(v["burst"])
			l.NoDelay = v["nodelay"] == "yes"
			l.Conn, _ = strconv.Atoi(v["conn"])
		}
		var err error
		if existing == nil {
			l.Name = v["name"]
			err = deps.RateLimits.Create(backgroundCtx(), l)
		} else {
			l.Name = existing.Name
			err = deps.RateLimits.Update(backgroundCtx(), l)
		}
		if err != nil {
			return newResultScreen(deps, title, false, err.Error()+includeWarning(deps), back()), nil
		}
		msg := fmt.Sprintf("Rate limit %s saved (%s).", l.Name, l.Describe())
		if existing == nil {
			msg += " Apply it from a host's \"Rate Limit\" menu."
		}
		return newResultScreen(deps, title, true, msg+includeWarning(deps)+realIPHint(deps), newRateLimitDetailScreen(deps, l.Name)), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}

func intRange(lo, hi int) func(string) error {
	return func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < lo || n > hi {
			return fmt.Errorf("enter a number between %d and %d", lo, hi)
		}
		return nil
	}
}

// --- Limit detail ---

type rateLimitDetailScreen struct {
	deps  Deps
	name  string
	limit ratelimit.Limit
	err   string
	menu  *simpleMenu
}

func newRateLimitDetailScreen(deps Deps, name string) *rateLimitDetailScreen {
	s := &rateLimitDetailScreen{deps: deps, name: name}
	l, err := deps.RateLimits.Get(name)
	if err != nil {
		s.err = err.Error()
	}
	s.limit = l
	s.menu = newSimpleMenu([]menuItem{
		{title: "Edit Rate Limit"},
		{title: "View Configuration"},
		{title: "View Hosts Using This Limit"},
		{title: "Delete Rate Limit"},
	})
	return s
}

func (s *rateLimitDetailScreen) Init() tea.Cmd { return nil }

func (s *rateLimitDetailScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
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

func (s *rateLimitDetailScreen) dispatch(choice int) (screen, tea.Cmd) {
	switch choice {
	case 0:
		l := s.limit
		return newRateLimitWizard(s.deps, &l), nil
	case 1:
		var parts []string
		for _, p := range []string{s.deps.RateLimits.ZonePath(s.name), s.deps.RateLimits.ApplyPath(s.name)} {
			data, err := os.ReadFile(p)
			if err != nil {
				return newResultScreen(s.deps, "View Configuration", false, err.Error(), s), nil
			}
			parts = append(parts, "# "+p+"\n"+string(data))
		}
		return newViewerScreen("Rate limit: "+s.name, strings.Join(parts, "\n"), s), nil
	case 2:
		users, err := s.deps.RateLimits.UsedBy(s.name)
		if err != nil {
			return newResultScreen(s.deps, "View Hosts", false, err.Error(), s), nil
		}
		content := "No hosts currently use this rate limit."
		if len(users) > 0 {
			content = "- " + strings.Join(users, "\n- ")
		}
		return newViewerScreen("Hosts using "+s.name, content, s), nil
	case 3:
		return newConfirmScreen("Delete Rate Limit", fmt.Sprintf("Delete rate limit %q?", s.name),
			func() (screen, tea.Cmd) {
				if err := s.deps.RateLimits.Delete(backgroundCtx(), s.name); err != nil {
					return newResultScreen(s.deps, "Delete Rate Limit", false, err.Error(), s), nil
				}
				return newResultScreen(s.deps, "Delete Rate Limit", true, "Rate limit deleted.", newRateLimitsScreen(s.deps)), nil
			},
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return s, nil
}

func (s *rateLimitDetailScreen) View(width, height int) string {
	body := headerStyle.Render("Rate Limit: "+s.name) + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err) + "\n\n"
	} else {
		body += "Limit: " + s.limit.Describe() + "\n"
		body += "Zone:  " + s.deps.RateLimits.ZonePath(s.name) + "\n"
		body += "Apply: " + s.deps.RateLimits.ApplyPath(s.name) + "\n\n"
	}
	body += s.menu.ViewFit(body, height)
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

// --- Per-host selection ---

// newHostRateLimitWizard applies a rate limit to a host or to one of its
// reverse-proxy locations.
func newHostRateLimitWizard(deps Deps, h nginx.ListedHost) screen {
	back := func() screen { return newHostDetailScreen(deps, h) }
	host, err := deps.Manager.GetHost(h.FileName)
	if err != nil {
		return newResultScreen(deps, "Rate Limit", false, err.Error(), back())
	}
	scopes := []string{ipScopeHost}
	current := map[string]string{ipScopeHost: host.RateLimit}
	for _, loc := range host.Locations {
		if loc.Proxy != nil {
			scope := "Path " + loc.Path
			scopes = append(scopes, scope)
			current[scope] = loc.RateLimit
		}
	}
	limits, _ := deps.RateLimits.List()
	options := []string{"(none)"}
	for _, l := range limits {
		options = append(options, l.Name)
	}
	fields := []wizardField{{Key: "scope", Label: "Apply to:", Kind: fieldChoice, Options: scopes}}
	for _, scope := range scopes {
		scope := scope
		fields = append(fields, wizardField{
			Key: "limit", Label: "Rate limit for " + strings.ToLower(scope[:1]) + scope[1:] + ":",
			Kind: fieldChoice, Options: options, ChoiceDefault: current[scope],
			ShowIf: func(v map[string]string) bool { return v["scope"] == scope },
		})
	}
	return newWizard("Rate Limit — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Rate Limit", false, err.Error(), back()), nil
		}
		limit := v["limit"]
		if limit == "(none)" {
			limit = ""
		}
		if v["scope"] == ipScopeHost {
			host.RateLimit = limit
		} else {
			path := strings.TrimPrefix(v["scope"], "Path ")
			for i := range host.Locations {
				if host.Locations[i].Path == path {
					host.Locations[i].RateLimit = limit
				}
			}
		}
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Rate Limit", false, err.Error(), back()), nil
		}
		return newResultScreen(deps, "Rate Limit", true, "Rate limit updated for "+h.ServerName+"."+realIPHint(deps), back()), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}

func rateLimitDesc(h nginx.ListedHost) string {
	var parts []string
	if h.RateLimit != "" {
		parts = append(parts, h.RateLimit)
	}
	for _, loc := range h.Locations {
		if loc.RateLimit != "" {
			parts = append(parts, loc.Path+": "+loc.RateLimit)
		}
	}
	if len(parts) == 0 {
		return mutedStyle.Render("(none)")
	}
	return strings.Join(parts, ", ")
}
