package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// liveLineMsg carries one line of progress/log output from a background run
// started by newLiveRunScreen.
type liveLineMsg string

// liveDoneMsg signals that a background run has finished.
type liveDoneMsg struct{ err error }

// visibleLogLines is how many lines of scrolling output are shown at once.
const visibleLogLines = 16

// liveRunScreen shows scrolling, real-time output from a long-running
// background action (certbot + Cloudflare API calls) instead of leaving the
// UI looking frozen while it works. The action runs in a goroutine started
// immediately on construction; run's emit callback is safe to call from any
// goroutine.
type liveRunScreen struct {
	title  string
	lines  []string
	offset int
	done   bool
	err    error
	ch     chan tea.Msg
	onDone func(err error) (screen, tea.Cmd)
}

func newLiveRunScreen(title string, run func(emit func(string)) error, onDone func(err error) (screen, tea.Cmd)) *liveRunScreen {
	ch := make(chan tea.Msg, 256)
	s := &liveRunScreen{title: title, ch: ch, onDone: onDone}
	go func() {
		err := run(func(line string) { ch <- liveLineMsg(line) })
		ch <- liveDoneMsg{err: err}
	}()
	return s
}

func (s *liveRunScreen) Init() tea.Cmd { return s.waitForMsg() }

func (s *liveRunScreen) waitForMsg() tea.Cmd {
	return func() tea.Msg { return <-s.ch }
}

func (s *liveRunScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch m := msg.(type) {
	case liveLineMsg:
		s.lines = append(s.lines, string(m))
		if s.offset = len(s.lines) - visibleLogLines; s.offset < 0 {
			s.offset = 0
		}
		return s, s.waitForMsg()
	case liveDoneMsg:
		s.done = true
		s.err = m.err
		return s, nil
	case tea.KeyMsg:
		if !s.done {
			return s, nil
		}
		switch m.String() {
		case "up", "k":
			if s.offset > 0 {
				s.offset--
			}
		case "down", "j":
			if s.offset < len(s.lines)-1 {
				s.offset++
			}
		case "enter", "esc", "q":
			return s.onDone(s.err)
		}
	}
	return s, nil
}

func (s *liveRunScreen) View(width, height int) string {
	body := headerStyle.Render(s.title) + "\n\n"

	end := s.offset + visibleLogLines
	if end > len(s.lines) {
		end = len(s.lines)
	}
	start := s.offset
	if start > end {
		start = end
	}
	body += mutedStyle.Render(strings.Join(s.lines[start:end], "\n"))

	var help [][2]string
	switch {
	case !s.done:
		body += "\n\n" + inputLabelStyle.Render("Working... this can take a few minutes, most of it spent waiting for DNS propagation.")
	case s.err != nil:
		body += "\n\n" + errorBoxStyle.Render("FAILED: "+s.err.Error())
		help = [][2]string{{"Enter", "Continue"}}
	default:
		body += "\n\n" + successBoxStyle.Render("Done.")
		help = [][2]string{{"Enter", "Continue"}}
	}
	if len(s.lines) > visibleLogLines {
		help = append([][2]string{{"↑↓", "Scroll"}}, help...)
	}
	return renderFrame(width, height, "GONIX", body, help)
}
