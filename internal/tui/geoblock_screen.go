package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/geoblock"
	"github.com/mrmmg/gonix/internal/nginx"
)

// geoScreen manages GeoIP setup and country lists.
type geoScreen struct {
	deps     Deps
	ready    bool
	database string
	lists    []geoblock.List
	menu     *simpleMenu
	err      string
}

func newGeoScreen(deps Deps) *geoScreen {
	s := &geoScreen{deps: deps}
	s.ready, s.database = deps.GeoBlock.Status()
	lists, err := deps.GeoBlock.List()
	if err != nil {
		s.err = err.Error()
	}
	s.lists = lists
	items := []menuItem{}
	if !s.ready {
		items = append(items, menuItem{title: "Set Up GeoIP", desc: "load " + deps.Config.GeoIP.Database, pinned: true})
	} else {
		items = append(items, menuItem{title: "+ Create Country List", pinned: true})
	}
	for _, l := range lists {
		items = append(items, menuItem{title: l.Name, desc: fmt.Sprintf("%s: %s", l.Mode, strings.Join(l.Countries, " "))})
	}
	s.menu = newSearchMenu(items, "name, mode or country")
	return s
}

func (s *geoScreen) Init() tea.Cmd { return nil }

func (s *geoScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
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
				if !s.ready {
					if err := s.deps.GeoBlock.SetUp(backgroundCtx()); err != nil {
						return newResultScreen(s.deps, "GeoIP", false, err.Error(), newGeoScreen(s.deps)), nil
					}
					return newResultScreen(s.deps, "GeoIP", true, "GeoIP lookups are set up."+includeWarning(s.deps), newGeoScreen(s.deps)), nil
				}
				return s, navPush(newCountryListWizard(s.deps, nil))
			}
			l := s.lists[s.menu.Selected()-1]
			return s, navPush(newCountryListDetailScreen(s.deps, l))
		}
	}
	return s, nil
}

func (s *geoScreen) View(width, height int) string {
	body := headerStyle.Render("Country Blocking") + "\n"
	body += mutedStyle.Render("Allow or refuse clients by country (needs the geoip2 module and a MaxMind database).") + "\n\n"
	if s.ready {
		body += enabledDot + " GeoIP ready, database " + s.database + "\n\n"
	} else {
		body += disabledDot + " GeoIP not set up\n\n"
	}
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.ViewFit(body, height)
	}
	help := searchHelp(s.menu, [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}})
	return renderFrame(width, height, "GONIX", body, help)
}

const (
	geoModeAllow = "Allowlist — only these countries may connect"
	geoModeBlock = "Blocklist — these countries are refused"
)

func newCountryListWizard(deps Deps, existing *geoblock.List) *wizardScreen {
	title := "Create Country List"
	mode, countries := geoModeBlock, ""
	var fields []wizardField
	if existing == nil {
		fields = append(fields, wizardField{
			Key: "name", Label: "List name (letters, digits and _):", Kind: fieldText, Placeholder: "eu_only",
			Validate: func(v string) error {
				if !nginx.ValidCountryListName(v) {
					return fmt.Errorf("name may only contain letters, digits and '_'")
				}
				if deps.GeoBlock.Exists(v) {
					return fmt.Errorf("a country list named %q already exists", v)
				}
				return nil
			},
		})
	} else {
		title = "Edit Country List — " + existing.Name
		countries = strings.Join(existing.Countries, " ")
		if existing.Mode == geoblock.Allowlist {
			mode = geoModeAllow
		}
	}
	fields = append(fields,
		wizardField{Key: "mode", Label: "List type:", Kind: fieldChoice, Options: []string{geoModeBlock, geoModeAllow}, ChoiceDefault: mode},
		wizardField{Key: "countries", Label: "Two-letter country codes (space separated, e.g. DE FR US):", Kind: fieldText,
			Default: countries, Validate: func(v string) error { _, err := geoblock.ParseCountries(v); return err }},
	)
	return newWizard(title, fields, func(v map[string]string) (screen, tea.Cmd) {
		l := geoblock.List{Mode: geoblock.Blocklist}
		if v["mode"] == geoModeAllow {
			l.Mode = geoblock.Allowlist
		}
		l.Countries, _ = geoblock.ParseCountries(v["countries"])
		l.Name = v["name"]
		if existing != nil {
			l.Name = existing.Name
		}
		if err := deps.GeoBlock.Save(backgroundCtx(), l, existing == nil); err != nil {
			return newResultScreen(deps, title, false, err.Error(), newGeoScreen(deps)), nil
		}
		msg := "Country list " + l.Name + " saved."
		if existing == nil {
			msg += " Apply it from a host's \"Country Access\" menu."
		}
		return newResultScreen(deps, title, true, msg+realIPHint(deps), newGeoScreen(deps)), nil
	}, func() (screen, tea.Cmd) { return newGeoScreen(deps), nil })
}

func newCountryListDetailScreen(deps Deps, l geoblock.List) screen {
	return newWizard("Country List — "+l.Name, []wizardField{
		{Key: "action", Label: fmt.Sprintf("%s: %s", l.Mode, strings.Join(l.Countries, " ")), Kind: fieldChoice,
			Options: []string{"Edit List", "Delete List"}},
	}, func(v map[string]string) (screen, tea.Cmd) {
		if v["action"] == "Edit List" {
			return newCountryListWizard(deps, &l), nil
		}
		if err := deps.GeoBlock.Delete(backgroundCtx(), l.Name); err != nil {
			return newResultScreen(deps, "Delete Country List", false, err.Error(), newGeoScreen(deps)), nil
		}
		return newResultScreen(deps, "Delete Country List", true, "Country list deleted.", newGeoScreen(deps)), nil
	}, func() (screen, tea.Cmd) { return newGeoScreen(deps), nil })
}

// newHostCountryWizard applies a country list to a host, or removes it.
func newHostCountryWizard(deps Deps, h nginx.ListedHost) screen {
	back := func() screen { return newHostDetailScreen(deps, h) }
	lists, _ := deps.GeoBlock.List()
	if len(lists) == 0 && h.CountryList == "" {
		return newResultScreen(deps, "Country Access", false, "No country lists exist yet. Create one under \"Country Blocking\" in the main menu.", back())
	}
	options := []string{"(none)"}
	for _, l := range lists {
		options = append(options, l.Name)
	}
	return newWizard("Country Access — "+h.ServerName, []wizardField{
		{Key: "list", Label: "Country list for " + h.ServerName + ":", Kind: fieldChoice, Options: options, ChoiceDefault: h.CountryList},
	}, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Country Access", false, err.Error(), back()), nil
		}
		host.CountryList = ""
		if v["list"] != "(none)" {
			host.CountryList = v["list"]
		}
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Country Access", false, err.Error(), back()), nil
		}
		return newResultScreen(deps, "Country Access", true, "Country access updated for "+h.ServerName+"."+realIPHint(deps), back()), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}
