package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// viewerScreen displays scrollable read-only text, used for configuration
// previews and log tails.
type viewerScreen struct {
	title  string
	lines  []string
	offset int
	next   screen // where Esc returns to; nil pops the stack
}

func newViewerScreen(title, content string, next screen) *viewerScreen {
	return &viewerScreen{title: title, lines: strings.Split(content, "\n"), next: next}
}

func (s *viewerScreen) Init() tea.Cmd { return nil }

func (s *viewerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			if s.offset > 0 {
				s.offset--
			}
		case "down", "j":
			if s.offset < len(s.lines)-1 {
				s.offset++
			}
		case "esc", "q", "enter":
			if s.next != nil {
				return s, navReplace(s.next)
			}
			return s, navPop()
		}
	}
	return s, nil
}

func (s *viewerScreen) View(width, height int) string {
	const visibleLines = 20
	end := s.offset + visibleLines
	if end > len(s.lines) {
		end = len(s.lines)
	}
	body := headerStyle.Render(s.title) + "\n\n"
	body += strings.Join(s.lines[s.offset:end], "\n")
	help := [][2]string{{"↑↓", "Scroll"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
