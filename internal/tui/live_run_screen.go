package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// liveLineMsg carries one line of progress/log output from a background run
// started by newLiveRunScreen.
type liveLineMsg string

// liveDoneMsg signals that a background run has finished.
type liveDoneMsg struct{ err error }

// liveRunScreen shows scrolling, real-time output from a long-running
// background action (certbot + Cloudflare API calls) instead of leaving the
// UI looking frozen while it works. The action runs in a goroutine started
// immediately on construction; run's emit callback is safe to call from any
// goroutine.
type liveRunScreen struct {
	title  string
	lines  []string
	offset int
	rows   int  // output lines that fit, from the last render
	follow bool // keep the newest output in view until the user scrolls up
	done   bool
	err    error
	ch     chan tea.Msg
	onDone func(err error) (screen, tea.Cmd)
}

func newLiveRunScreen(title string, run func(emit func(string)) error, onDone func(err error) (screen, tea.Cmd)) *liveRunScreen {
	ch := make(chan tea.Msg, 256)
	s := &liveRunScreen{title: title, ch: ch, onDone: onDone, follow: true, rows: 16}
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
			s.offset, s.follow = max(0, s.offset-1), false
		case "down", "j":
			s.offset++
			s.follow = s.offset >= len(s.lines)-s.rows
		case "enter", "esc", "q":
			return s.onDone(s.err)
		}
	}
	return s, nil
}

func (s *liveRunScreen) View(width, height int) string {
	body := headerStyle.Render(s.title) + "\n\n"

	// Rows left after the title and the status lines below the output.
	s.rows = max(1, height-frameChrome-(lipgloss.Height(body)-1)-4)
	maxOffset := max(0, len(s.lines)-s.rows)
	if s.follow || s.offset > maxOffset {
		s.offset = maxOffset
	}
	end := min(s.offset+s.rows, len(s.lines))
	body += mutedStyle.Render(strings.Join(fitLines(s.lines[s.offset:end], width-4), "\n"))

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
	if len(s.lines) > s.rows {
		help = append([][2]string{{"↑↓", "Scroll"}}, help...)
	}
	return renderFrame(width, height, "GONIX", body, help)
}
