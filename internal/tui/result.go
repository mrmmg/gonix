package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// resultScreen shows the outcome of an action (success or failure) and
// returns to a follow-up screen when dismissed.
type resultScreen struct {
	deps    Deps
	title   string
	ok      bool
	message string
	next    screen
}

func newResultScreen(deps Deps, title string, ok bool, message string, next screen) *resultScreen {
	return &resultScreen{deps: deps, title: title, ok: ok, message: message, next: next}
}

func (s *resultScreen) Init() tea.Cmd { return nil }

func (s *resultScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter", "esc", "q":
			return s, navReplace(s.next)
		}
	}
	return s, nil
}

func (s *resultScreen) View(width, height int) string {
	body := headerStyle.Render(s.title) + "\n\n"
	if s.ok {
		body += successBoxStyle.Render(s.message)
	} else {
		body += errorBoxStyle.Render("Error: " + s.message)
	}
	help := [][2]string{{"Enter", "Continue"}}
	return renderFrame(width, height, "NGINX MANAGER", body, help)
}
