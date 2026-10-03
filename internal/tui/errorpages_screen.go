package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/errorpages"
	"github.com/mrmmg/gonix/internal/nginx"
)

type errorPagesScreen struct {
	deps     Deps
	snippets []errorpages.Snippet
	menu     *simpleMenu
	err      string
}

func newErrorPagesScreen(deps Deps) *errorPagesScreen {
	s := &errorPagesScreen{deps: deps}
	s.reload()
	return s
}

func (s *errorPagesScreen) reload() {
	snippets, err := s.deps.ErrorPages.Store.List()
	if err != nil {
		s.err = err.Error()
	}
	s.snippets = snippets
	items := make([]menuItem, 0, len(snippets)+1)
	items = append(items, menuItem{title: "+ Create Error Pages Snippet"})
	for _, sn := range snippets {
		target := sn.Directory
		if sn.IsTemplate() {
			target = sn.PagePath(0) + " (template)"
		}
		items = append(items, menuItem{title: sn.Name, desc: "codes " + errorpages.FormatCodes(sn.Codes) + " → " + target})
	}
	s.menu = newSimpleMenu(items)
}

func (s *errorPagesScreen) Init() tea.Cmd { return nil }

func (s *errorPagesScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			if s.menu.Selected() == 0 {
				return s, navPush(newErrorPagesWizard(s.deps, nil))
			}
			sn := s.snippets[s.menu.Selected()-1]
			return s, navPush(newErrorPagesDetailScreen(s.deps, sn.Name))
		}
	}
	return s, nil
}

func (s *errorPagesScreen) View(width, height int) string {
	body := headerStyle.Render("Error Pages") + "\n"
	body += mutedStyle.Render("Snippets in "+s.deps.ErrorPages.Store.Directory+", included by hosts at server level.") + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.View()
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

const (
	modePerCode  = "Separate page per code (<code>.html)"
	modeTemplate = "Single template, code injected via sub_filter"

	perCodeCodesDefault  = "502, 503, 504"
	templateCodesDefault = "400, 401, 403, 404, 413, 429, 500, 502, 503, 504"
)

func isTemplateMode(v map[string]string) bool { return v["mode"] == modeTemplate }

// newErrorPagesWizard creates a new snippet when existing is nil, otherwise
// edits existing (its name is kept; everything else is asked again).
func newErrorPagesWizard(deps Deps, existing *errorpages.Snippet) *wizardScreen {
	title := "Create Error Pages Snippet"
	dirDefault := deps.Config.ErrorPages.PagesDirectory
	modeDefault := modePerCode
	perCodeCodes, templateCodes := perCodeCodesDefault, templateCodesDefault
	templateDefault, placeholderDefault := errorpages.DefaultTemplate, errorpages.DefaultPlaceholder
	var fields []wizardField
	if existing == nil {
		fields = append(fields, wizardField{
			Key: "name", Label: "Snippet name (letters, digits, - and _; e.g. default):", Kind: fieldText,
			Placeholder: "default",
			Validate: func(v string) error {
				if !nginx.ValidSnippetName(v) {
					return fmt.Errorf("name may only contain letters, digits, '-' and '_'")
				}
				if deps.ErrorPages.Store.Exists(v) {
					return fmt.Errorf("a snippet named %q already exists", v)
				}
				return nil
			},
		})
	} else {
		title = "Edit Error Pages — " + existing.Name
		dirDefault = existing.Directory
		if existing.IsTemplate() {
			modeDefault = modeTemplate
			templateCodes = errorpages.FormatCodes(existing.Codes)
			templateDefault, placeholderDefault = existing.Template, existing.Placeholder
		} else {
			perCodeCodes = errorpages.FormatCodes(existing.Codes)
		}
	}
	validateCodes := func(v string) error { _, err := errorpages.ParseCodes(v); return err }
	fields = append(fields,
		wizardField{
			Key: "mode", Label: "How are pages organized?", Kind: fieldChoice,
			Options: []string{modePerCode, modeTemplate}, ChoiceDefault: modeDefault,
		},
		wizardField{
			Key: "directory", Label: "Directory holding the HTML page(s):", Kind: fieldText,
			Default: dirDefault, Validate: errorpages.ValidateDirectory,
		},
		wizardField{
			Key: "template", Label: "Template file name inside that directory:", Kind: fieldText,
			Placeholder: errorpages.DefaultTemplate, Default: templateDefault,
			Validate: errorpages.ValidateTemplateFile, ShowIf: isTemplateMode,
		},
		wizardField{
			Key: "placeholder", Label: "Placeholder text replaced with the status code:", Kind: fieldText,
			Placeholder: errorpages.DefaultPlaceholder, Default: placeholderDefault,
			Validate: errorpages.ValidatePlaceholder, ShowIf: isTemplateMode,
		},
		wizardField{
			Key: "codes", Label: "Error codes to handle (comma separated; one <code>.html each):", Kind: fieldText,
			Placeholder: perCodeCodesDefault, Default: perCodeCodes, Validate: validateCodes,
			ShowIf: func(v map[string]string) bool { return !isTemplateMode(v) },
		},
		wizardField{
			Key: "codes", Label: "Error codes to handle (comma separated):", Kind: fieldText,
			Placeholder: templateCodesDefault, Default: templateCodes, Validate: validateCodes,
			ShowIf: isTemplateMode,
		},
	)

	back := func() screen {
		if existing == nil {
			return newErrorPagesScreen(deps)
		}
		return newErrorPagesDetailScreen(deps, existing.Name)
	}
	return newWizard(title, fields, func(v map[string]string) (screen, tea.Cmd) {
		codes, _ := errorpages.ParseCodes(v["codes"])
		sn := errorpages.Snippet{Directory: strings.TrimSuffix(v["directory"], "/"), Codes: codes}
		if isTemplateMode(v) {
			sn.Template, sn.Placeholder = v["template"], v["placeholder"]
		}
		var err error
		if existing == nil {
			sn.Name = v["name"]
			err = deps.ErrorPages.Create(backgroundCtx(), sn)
		} else {
			sn.Name = existing.Name
			err = deps.ErrorPages.Update(backgroundCtx(), sn)
		}
		if err != nil {
			return newResultScreen(deps, title, false, err.Error(), back()), nil
		}
		msg := "Snippet saved to " + deps.ErrorPages.Store.Path(sn.Name) + "."
		if existing == nil {
			msg += " Apply it to hosts from each host's \"Error Pages\" menu."
		}
		if missing := sn.MissingPages(); len(missing) > 0 {
			msg += "\n\nWarning: these pages do not exist yet, Nginx will show its default page for them:\n  " +
				strings.Join(missing, "\n  ")
		}
		if sn.PlaceholderMissing() {
			msg += fmt.Sprintf("\n\nWarning: %s does not contain the placeholder %q, so the status code will not appear in the page.",
				sn.PagePath(0), sn.Placeholder)
		}
		return newResultScreen(deps, title, true, msg, newErrorPagesDetailScreen(deps, sn.Name)), nil
	}, func() (screen, tea.Cmd) { return back(), nil })
}

// --- Snippet detail ---

type errorPagesDetailScreen struct {
	deps Deps
	name string
	sn   errorpages.Snippet
	err  string
	menu *simpleMenu
}

const (
	epdEdit = iota
	epdView
	epdViewHosts
	epdDelete
)

func newErrorPagesDetailScreen(deps Deps, name string) *errorPagesDetailScreen {
	s := &errorPagesDetailScreen{deps: deps, name: name}
	sn, err := deps.ErrorPages.Store.Get(name)
	if err != nil {
		s.err = err.Error()
	}
	s.sn = sn
	s.menu = newSimpleMenu([]menuItem{
		{title: "Edit Snippet"},
		{title: "View Snippet"},
		{title: "View Hosts Using This Snippet"},
		{title: "Delete Snippet"},
	})
	return s
}

func (s *errorPagesDetailScreen) Init() tea.Cmd { return nil }

func (s *errorPagesDetailScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
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

func (s *errorPagesDetailScreen) dispatch(choice int) (screen, tea.Cmd) {
	switch choice {
	case epdEdit:
		sn := s.sn
		return newErrorPagesWizard(s.deps, &sn), nil
	case epdView:
		data, err := os.ReadFile(s.deps.ErrorPages.Store.Path(s.name))
		if err != nil {
			return newResultScreen(s.deps, "View Snippet", false, err.Error(), s), nil
		}
		return newViewerScreen("Snippet: "+s.name, string(data), s), nil
	case epdViewHosts:
		users, err := s.deps.ErrorPages.UsedBy(s.name)
		if err != nil {
			return newResultScreen(s.deps, "View Hosts", false, err.Error(), s), nil
		}
		content := "No hosts currently use this snippet."
		if len(users) > 0 {
			content = "- " + strings.Join(users, "\n- ")
		}
		return newViewerScreen("Hosts using "+s.name, content, s), nil
	case epdDelete:
		return newConfirmScreen("Delete Snippet",
			fmt.Sprintf("Delete error pages snippet %q? The HTML pages themselves are not touched.", s.name),
			func() (screen, tea.Cmd) {
				if err := s.deps.ErrorPages.Delete(backgroundCtx(), s.name); err != nil {
					return newResultScreen(s.deps, "Delete Snippet", false, err.Error(), s), nil
				}
				return newResultScreen(s.deps, "Delete Snippet", true, "Snippet deleted.", newErrorPagesScreen(s.deps)), nil
			},
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return s, nil
}

func (s *errorPagesDetailScreen) View(width, height int) string {
	body := headerStyle.Render("Error Pages: "+s.name) + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err) + "\n\n"
	} else {
		body += "File:      " + s.deps.ErrorPages.Store.Path(s.name) + "\n"
		body += "Directory: " + s.sn.Directory + "\n"
		if s.sn.IsTemplate() {
			p := s.sn.PagePath(0)
			status := enabledDot + " " + p
			switch {
			case len(s.sn.MissingPages()) > 0:
				status = disabledDot + " " + p + mutedStyle.Render("  (missing)")
			case s.sn.PlaceholderMissing():
				status = disabledDot + " " + p + mutedStyle.Render("  (placeholder not found)")
			}
			body += "Template:  " + status + "\n"
			body += "Placeholder: " + s.sn.Placeholder + "\n"
			body += "Codes:     " + errorpages.FormatCodes(s.sn.Codes) + "\n"
		} else {
			body += "Pages:\n"
			for _, c := range s.sn.Codes {
				p := s.sn.PagePath(c)
				status := enabledDot + " " + p
				if _, err := os.Stat(p); err != nil {
					status = disabledDot + " " + p + mutedStyle.Render("  (missing)")
				}
				body += fmt.Sprintf("  %d  %s\n", c, status)
			}
		}
		body += "\n"
	}
	body += s.menu.View()
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

// --- Per-host selection ---

// errorPagesOptions returns the choices offered when picking a snippet for
// a host: "(none)" followed by every existing snippet name.
func errorPagesOptions(deps Deps) []string {
	options := []string{"(none)"}
	snippets, _ := deps.ErrorPages.Store.List()
	for _, sn := range snippets {
		options = append(options, sn.Name)
	}
	return options
}

// newHostErrorPagesScreen lets the operator choose which error pages
// snippet a host includes, or none.
func newHostErrorPagesScreen(deps Deps, h nginx.ListedHost) *wizardScreen {
	fields := []wizardField{
		{Key: "snippet", Label: "Error pages snippet for " + h.ServerName + ":", Kind: fieldChoice,
			Options: errorPagesOptions(deps), ChoiceDefault: h.ErrorPagesSnippet},
	}
	return newWizard("Error Pages — "+h.ServerName, fields, func(v map[string]string) (screen, tea.Cmd) {
		host, err := deps.Manager.GetHost(h.FileName)
		if err != nil {
			return newResultScreen(deps, "Error Pages", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		host.ErrorPagesSnippet = ""
		if v["snippet"] != "(none)" {
			host.ErrorPagesSnippet = v["snippet"]
		}
		if _, err := deps.HostService.UpdateHost(backgroundCtx(), host); err != nil {
			return newResultScreen(deps, "Error Pages", false, err.Error(), newHostDetailScreen(deps, h)), nil
		}
		return newResultScreen(deps, "Error Pages", true, "Error pages updated for "+h.ServerName+".", newHostDetailScreen(deps, h)), nil
	}, func() (screen, tea.Cmd) { return newHostDetailScreen(deps, h), nil })
}
