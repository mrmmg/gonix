package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/app"
	"github.com/mrmmg/gonix/internal/lab"
)

// The lab (see internal/lab) is a second, separate Nginx for trying GoNix.
// Its menu is offered only when GONIX_LAB is set. "Enter Lab" switches the
// whole TUI to the lab's GoNix instance until "Exit Lab".

// inLab is set while the TUI works on the lab instead of the real server.
var inLab bool

// labEnterMsg asks the root model to switch to the lab's services.
type labEnterMsg struct{ deps Deps }

// labExitMsg asks the root model to switch back to the real server.
type labExitMsg struct{}

// buildLabDeps wires the services of the lab instance; replaced in tests.
var buildLabDeps = func(l *lab.Lab) (Deps, error) {
	svc, err := app.Build(l.InstanceFile)
	if err != nil {
		return Deps{}, err
	}
	return *svc, nil
}

// newLab returns the lab to manage; replaced in tests.
var newLab = lab.New

const (
	laSetup = iota
	laEnter
	laAddHost
	laStartStop
	laGuide
	laRemove
)

type labScreen struct {
	deps    Deps
	lab     *lab.Lab
	state   *lab.State
	active  bool
	loadErr error
	menu    *simpleMenu
	actions []int
}

func newLabScreen(deps Deps) *labScreen {
	s := &labScreen{deps: deps, lab: newLab()}
	s.refresh()
	return s
}

func (s *labScreen) refresh() {
	s.state, s.loadErr = s.lab.Load()
	s.active = s.state != nil && s.lab.Active(backgroundCtx())
	var items []menuItem
	s.actions = s.actions[:0]
	add := func(action int, title, desc string) {
		items = append(items, menuItem{title: title, desc: desc})
		s.actions = append(s.actions, action)
	}
	if s.state == nil {
		add(laSetup, "Set Up Lab", "a separate Nginx with hand-written test hosts")
	} else {
		add(laEnter, "Enter Lab", "work on the lab with every GoNix screen")
		add(laAddHost, "Add Test Host", "write a hand-written host of a chosen kind")
		if s.active {
			add(laStartStop, "Stop Lab Nginx", "")
		} else {
			add(laStartStop, "Start Lab Nginx", "")
		}
		add(laSetup, "Reset Lab", "start over: delete the lab and set it up again")
	}
	add(laGuide, "Usage Guide", "how the lab works and what to try")
	if s.state != nil {
		add(laRemove, "Remove Lab", "stop it and delete everything it created")
	}
	if s.menu == nil {
		s.menu = newSimpleMenu(items)
	} else {
		s.menu.SetItems(items)
	}
}

func (s *labScreen) Init() tea.Cmd { return nil }

func (s *labScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	if s.menu.HandleKey(km) {
		return s, nil
	}
	switch km.String() {
	case "esc", "q":
		return s, navPop()
	case "enter":
		if i := s.menu.Selected(); i < len(s.actions) {
			return s.dispatch(s.actions[i])
		}
	}
	return s, nil
}

func (s *labScreen) back() (screen, tea.Cmd) { return newLabScreen(s.deps), nil }

func (s *labScreen) dispatch(action int) (screen, tea.Cmd) {
	ctx := backgroundCtx()
	switch action {
	case laSetup:
		if s.state == nil {
			return s.setupWizard(), nil
		}
		return newConfirmScreen("Reset Lab",
			fmt.Sprintf("Delete everything in %s and set the lab up again?", s.state.Dir),
			func() (screen, tea.Cmd) { return s.setupWizard(), nil }, s.back), nil
	case laEnter:
		deps, err := buildLabDeps(s.lab)
		if err != nil {
			return newResultScreen(s.deps, "Enter Lab", false, err.Error(), s), nil
		}
		return s, func() tea.Msg { return labEnterMsg{deps} }
	case laAddHost:
		return s.addHostWizard(), nil
	case laStartStop:
		var err error
		if s.active {
			err = s.lab.Stop(ctx)
		} else {
			err = s.lab.Start(ctx)
		}
		if err != nil {
			return newResultScreen(s.deps, "Lab Nginx", false, err.Error(), newLabScreen(s.deps)), nil
		}
		s.refresh()
		return s, nil
	case laGuide:
		return newViewerScreen("Lab Guide", s.lab.Guide(ctx, s.state), s), nil
	case laRemove:
		return newConfirmScreen("Remove Lab",
			fmt.Sprintf("Stop the lab's Nginx and delete %s, its systemd unit and the %q instance?", s.state.Dir, lab.Instance),
			func() (screen, tea.Cmd) {
				if err := s.lab.Remove(ctx); err != nil {
					return newResultScreen(s.deps, "Remove Lab", false, err.Error(), newLabScreen(s.deps)), nil
				}
				return newResultScreen(s.deps, "Remove Lab", true, "The lab was removed.", newLabScreen(s.deps)), nil
			}, s.back), nil
	}
	return s, nil
}

func (s *labScreen) setupWizard() *wizardScreen {
	dir, httpPort, httpsPort := lab.DefaultDir, lab.DefaultHTTPPort, lab.DefaultHTTPSPort
	if s.state != nil {
		dir, httpPort, httpsPort = s.state.Dir, s.state.HTTPPort, s.state.HTTPSPort
	}
	fields := []wizardField{
		{Key: "dir", Label: "Lab directory (created; must be new or empty):", Kind: fieldText, Default: dir},
		{Key: "http", Label: "HTTP port of the lab's Nginx:", Kind: fieldText, Default: strconv.Itoa(httpPort), Validate: validatePort},
		{Key: "https", Label: "HTTPS port of the lab's Nginx:", Kind: fieldText, Default: strconv.Itoa(httpsPort), Validate: validatePort},
		{Key: "samples", Label: "Add one test host of every kind?", Kind: fieldBool, BoolDefault: true},
	}
	return newWizard("Set Up Lab", fields, func(v map[string]string) (screen, tea.Cmd) {
		httpP, _ := strconv.Atoi(v["http"])
		httpsP, _ := strconv.Atoi(v["https"])
		st, err := s.lab.Setup(backgroundCtx(), lab.Options{Dir: v["dir"], HTTPPort: httpP, HTTPSPort: httpsP, Samples: v["samples"] == "yes"})
		if err != nil {
			return newResultScreen(s.deps, "Set Up Lab", false, err.Error(), newLabScreen(s.deps)), nil
		}
		msg := fmt.Sprintf("The lab is running in %s on ports %d/%d with %d test host(s).\n\nNext: \"Enter Lab\", or read the \"Usage Guide\".",
			st.Dir, st.HTTPPort, st.HTTPSPort, len(st.Hosts))
		return newResultScreen(s.deps, "Set Up Lab", true, msg, newLabScreen(s.deps)), nil
	}, s.back)
}

func (s *labScreen) addHostWizard() *wizardScreen {
	options := make([]string, len(lab.Scenarios))
	byOption := map[string]lab.Scenario{}
	for i, sc := range lab.Scenarios {
		options[i] = sc.Title
		byOption[sc.Title] = sc
	}
	fields := []wizardField{
		{Key: "kind", Label: "Kind of hand-written host:", Kind: fieldChoice, Options: options},
		{Key: "domain", Label: "Domain (empty for the suggested one):", Kind: fieldText,
			ShowIf: func(v map[string]string) bool { return byOption[v["kind"]].NeedsDomain() }},
	}
	return newWizard("Add Test Host", fields, func(v map[string]string) (screen, tea.Cmd) {
		sc := byOption[v["kind"]]
		domain := v["domain"]
		if domain == "" {
			domain = sc.DefaultDomain
		}
		h, err := s.lab.AddHost(backgroundCtx(), s.state, sc.ID, domain)
		if err != nil {
			return newResultScreen(s.deps, "Add Test Host", false, err.Error(), newLabScreen(s.deps)), nil
		}
		msg := fmt.Sprintf("Wrote and enabled %s (%s).\n\nWhen adopted: %s.", h.File, sc.Title, sc.Adopt)
		return newResultScreen(s.deps, "Add Test Host", true, msg, newLabScreen(s.deps)), nil
	}, s.back)
}

func (s *labScreen) View(width, height int) string {
	body := headerStyle.Render("Lab") + "\n"
	switch {
	case s.loadErr != nil:
		body += dangerStyle.Render(s.loadErr.Error()) + "\n"
	case s.state == nil:
		body += mutedStyle.Render("No lab yet. A lab is a second Nginx, separate from the system's, for trying GoNix safely.") + "\n"
	default:
		state := "stopped"
		if s.active {
			state = "running"
		}
		body += fmt.Sprintf("%s · Nginx %s on %d/%d · %d test host(s)\n", s.state.Dir, state, s.state.HTTPPort, s.state.HTTPSPort, len(s.state.Hosts))
	}
	body += "\n" + s.menu.ViewFit(body, height)
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
