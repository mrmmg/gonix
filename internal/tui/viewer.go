package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// viewerScreen displays scrollable read-only text, used for configuration
// previews and log tails. It uses the whole height of the terminal.
type viewerScreen struct {
	title  string
	lines  []string
	offset int
	rows   int    // lines that fit, from the last render
	next   screen // where Esc returns to; nil pops the stack
}

func newViewerScreen(title, content string, next screen) *viewerScreen {
	return &viewerScreen{title: title, lines: strings.Split(strings.TrimRight(content, "\n"), "\n"), next: next, rows: 20}
}

func (s *viewerScreen) Init() tea.Cmd { return nil }

func (s *viewerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			s.offset--
		case "down", "j":
			s.offset++
		case "pgup", "b":
			s.offset -= s.rows
		case "pgdown", "f", " ":
			s.offset += s.rows
		case "home", "g":
			s.offset = 0
		case "end", "G":
			s.offset = len(s.lines)
		case "esc", "q", "enter":
			if s.next != nil {
				return s, navReplace(s.next)
			}
			return s, navPop()
		}
		s.clamp()
	}
	return s, nil
}

// clamp keeps the offset where the last page is still full.
func (s *viewerScreen) clamp() {
	s.offset = max(0, min(s.offset, len(s.lines)-s.rows))
}

func (s *viewerScreen) View(width, height int) string {
	body := headerStyle.Render(s.title) + "\n\n"
	// Rows left after the title and the position line.
	s.rows = max(1, height-frameChrome-(lipgloss.Height(body)-1)-1)
	s.clamp()
	end := min(s.offset+s.rows, len(s.lines))
	body += strings.Join(fitLines(s.lines[s.offset:end], width-4), "\n")

	help := [][2]string{{"Esc", "Back"}}
	if len(s.lines) > s.rows {
		pos := fmt.Sprintf("lines %d–%d of %d", s.offset+1, end, len(s.lines))
		if rest := len(s.lines) - end; rest > 0 {
			pos += fmt.Sprintf(" · ↓ %d more", rest)
		}
		body += "\n" + strings.Repeat("\n", s.rows-(end-s.offset)) + mutedStyle.Render(pos)
		help = append([][2]string{{"↑↓", "Scroll"}, {"PgUp/PgDn", "Page"}, {"Home/End", "Top/Bottom"}}, help...)
	}
	return renderFrame(width, height, "GONIX", body, help)
}

// fitLines cuts lines to width, so a long line never wraps into the rows
// counted for the next ones.
func fitLines(lines []string, width int) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Truncate(l, max(1, width), "…")
	}
	return out
}
