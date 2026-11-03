package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/iplist"
	"github.com/mrmmg/gonix/internal/logs"
	"github.com/mrmmg/gonix/internal/nginx"
)

// logTailLines is how much of the end of a log is read on each refresh.
const logTailLines = 2000

// logsTickMsg refreshes the live log view.
type logsTickMsg struct{ id int }

// liveLogsScreen tails a host's access (or error) log, with filtering, a
// traffic summary, and blocking a client straight from a log line.
type liveLogsScreen struct {
	deps    Deps
	host    nginx.ListedHost
	id      int // ignores ticks from a previous instance of this screen
	errLog  bool
	summary bool
	paused  bool
	filter  logs.Filter
	editing bool
	input   textinput.Model
	entries []logs.Entry // access log entries matching filter, oldest first
	lines   []string     // error log lines
	cursor  int          // index into entries, from the newest
	status  string
	err     string
}

var liveLogsSeq int

func newLiveLogsScreen(deps Deps, h nginx.ListedHost) *liveLogsScreen {
	s := &liveLogsScreen{deps: deps, host: h}
	s.input = textinput.New()
	s.input.Placeholder = "status:5xx ip:1.2.3.4 path:/api"
	s.input.Width = 50
	s.reload()
	return s
}

func (s *liveLogsScreen) tick() tea.Cmd {
	id := s.id
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return logsTickMsg{id} })
}

// Init (re)starts refreshing. Each start gets a new id, so a tick still
// pending from before the screen was covered by a dialog is ignored instead
// of doubling the refresh rate.
func (s *liveLogsScreen) Init() tea.Cmd { return s.resume() }

func (s *liveLogsScreen) resume() tea.Cmd {
	liveLogsSeq++
	s.id = liveLogsSeq
	s.reload()
	return s.tick()
}

func (s *liveLogsScreen) path() string {
	if s.errLog {
		return logs.ErrorLogPath(s.deps.Config.Logs.NginxDirectory, s.host.ServerName)
	}
	return logs.AccessLogPath(s.deps.Config.Logs.NginxDirectory, s.host.ServerName)
}

func (s *liveLogsScreen) reload() {
	lines, err := logs.Tail(s.path(), logTailLines)
	if err != nil {
		s.err = err.Error()
		s.entries, s.lines = nil, nil
		return
	}
	s.err = ""
	if s.errLog {
		s.lines = lines
		return
	}
	s.entries = s.entries[:0]
	for _, l := range lines {
		if e, ok := logs.ParseAccessLine(l); ok && s.filter.Match(e) {
			s.entries = append(s.entries, e)
		}
	}
	if s.cursor >= len(s.entries) {
		s.cursor = max(len(s.entries)-1, 0)
	}
}

// selected returns the entry under the cursor (counted from the newest).
func (s *liveLogsScreen) selected() (logs.Entry, bool) {
	if s.errLog || len(s.entries) == 0 {
		return logs.Entry{}, false
	}
	return s.entries[len(s.entries)-1-s.cursor], true
}

func (s *liveLogsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case logsTickMsg:
		if msg.id != s.id {
			return s, nil
		}
		if !s.paused && !s.editing {
			s.reload()
		}
		return s, s.tick()
	case tea.KeyMsg:
		if s.editing {
			return s.updateFilter(msg)
		}
		switch msg.String() {
		case "esc", "q":
			s.id = -1 // stop ticking
			return s, navPop()
		case "up", "k":
			if s.cursor < len(s.entries)-1 {
				s.cursor++
			}
		case "down", "j":
			if s.cursor > 0 {
				s.cursor--
			}
		case "/":
			s.editing = true
			s.input.SetValue(s.filter.String())
			s.input.CursorEnd()
			s.input.Focus()
			return s, textinput.Blink
		case "s":
			s.summary = !s.summary
		case "e":
			s.errLog = !s.errLog
			s.cursor = 0
			s.reload()
		case " ", "p":
			s.paused = !s.paused
		case "f":
			if e, ok := s.selected(); ok {
				s.filter = logs.Filter{IP: e.IP}
				s.cursor = 0
				s.reload()
			}
		case "b":
			if e, ok := s.selected(); ok {
				return s.confirmBlock(e.IP)
			}
		}
	}
	return s, nil
}

func (s *liveLogsScreen) updateFilter(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		s.editing = false
		s.status = ""
		return s, nil
	case "enter":
		f, err := logs.ParseFilter(s.input.Value())
		if err != nil {
			s.status = err.Error()
			return s, nil
		}
		s.filter, s.editing, s.status, s.cursor = f, false, "", 0
		s.reload()
		return s, nil
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return s, cmd
}

func (s *liveLogsScreen) confirmBlock(ip string) (screen, tea.Cmd) {
	return newConfirmScreen("Block "+ip,
		fmt.Sprintf("Refuse every request from %s to %s?", ip, s.host.ServerName),
		func() (screen, tea.Cmd) {
			msg, err := blockIP(s.deps, s.host.FileName, ip)
			if err != nil {
				return newResultScreen(s.deps, "Block "+ip, false, err.Error(), s), nil
			}
			return newResultScreen(s.deps, "Block "+ip, true, msg+realIPHint(s.deps), s), nil
		},
		func() (screen, tea.Cmd) { return s, s.resume() },
	), nil
}

// blockedListName is the blocklist "Block IP" creates for a host that has
// no IP access list yet.
const blockedListName = "blocked"

// blockIP refuses ip on the host: it is added to the host's blocklist, or
// to a "blocked" list that is created and applied when the host has none.
// An allowlist cannot express "everyone except ip", so it is refused.
func blockIP(deps Deps, fileName, ip string) (string, error) {
	ctx := backgroundCtx()
	host, err := deps.Manager.GetHost(fileName)
	if err != nil {
		return "", err
	}
	if host.IPListName != "" {
		l, err := deps.IPLists.Store.Get(host.IPListName)
		if err != nil {
			return "", err
		}
		if l.Mode == iplist.Allowlist {
			if l.Contains(ip) {
				return "", fmt.Errorf("%s is admitted by the allowlist %q; remove it from that list instead", ip, l.Name)
			}
			return "", fmt.Errorf("%s is already refused: the allowlist %q does not include it", ip, l.Name)
		}
		if err := deps.IPLists.AddEntry(ctx, l.Name, ip); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s added to the blocklist %q.", ip, l.Name), nil
	}

	if deps.IPLists.Store.Exists(blockedListName) {
		l, err := deps.IPLists.Store.Get(blockedListName)
		if err != nil {
			return "", err
		}
		if l.Mode != iplist.Blocklist {
			return "", fmt.Errorf("the IP access list %q exists but is not a blocklist", blockedListName)
		}
		if err := deps.IPLists.AddEntry(ctx, blockedListName, ip); err != nil {
			return "", err
		}
	} else if err := deps.IPLists.Create(ctx, iplist.List{Name: blockedListName, Mode: iplist.Blocklist, Entries: []string{ip}}); err != nil {
		return "", err
	}
	host.IPListName = blockedListName
	if _, err := deps.HostService.UpdateHost(ctx, host); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s added to the blocklist %q, now applied to %s.", ip, blockedListName, host.ServerName), nil
}

func (s *liveLogsScreen) View(width, height int) string {
	kind := "Access log"
	if s.errLog {
		kind = "Error log"
	}
	state := enabledDot + " live"
	if s.paused {
		state = disabledDot + " paused"
	}
	body := headerStyle.Render(kind+": "+s.host.ServerName) + "  " + state + "\n"
	body += mutedStyle.Render(s.path()) + "\n"
	if !s.errLog {
		f := s.filter.String()
		if f == "" {
			f = "(none)"
		}
		body += "Filter: " + f + "\n"
	}
	if s.editing {
		body += focusedInputStyle.Render(s.input.View()) + "\n"
	}
	if s.status != "" {
		body += s.status + "\n"
	}
	body += "\n"

	rows := max(height-14, 5)
	switch {
	case s.err != "":
		body += errorBoxStyle.Render(s.err)
	case s.errLog:
		start := max(len(s.lines)-rows, 0)
		body += strings.Join(s.lines[start:], "\n")
	case s.summary:
		body += renderSummary(logs.Summarize(s.entries, 5))
	default:
		body += s.renderEntries(rows, width)
	}

	help := [][2]string{{"↑↓", "Select"}, {"/", "Filter"}, {"f", "Only this IP"}, {"b", "Block IP"},
		{"s", "Summary"}, {"e", "Error log"}, {"p", "Pause"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}

func (s *liveLogsScreen) renderEntries(rows, width int) string {
	if len(s.entries) == 0 {
		return mutedStyle.Render("No matching requests yet.")
	}
	var b strings.Builder
	// Newest first; keep the cursor visible.
	first := 0
	if s.cursor >= rows {
		first = s.cursor - rows + 1
	}
	for i := first; i < len(s.entries) && i < first+rows; i++ {
		e := s.entries[len(s.entries)-1-i]
		line := fmt.Sprintf("%s  %-15s %3d %-6s %s", e.Time.Format("15:04:05"), e.IP, e.Status, e.Method, e.Path)
		if w := width - 8; w > 20 && len(line) > w {
			line = line[:w]
		}
		style := normalItemStyle
		switch {
		case i == s.cursor:
			style = selectedItemStyle
		case e.Status >= 500:
			style = dangerStyle
		}
		b.WriteString(style.Render(line) + "\n")
	}
	return b.String()
}

func renderSummary(sum logs.Summary) string {
	var b strings.Builder
	pct := func(n int) float64 {
		if sum.Total == 0 {
			return 0
		}
		return 100 * float64(n) / float64(sum.Total)
	}
	fmt.Fprintf(&b, "Requests: %d (last %d log lines)", sum.Total, logTailLines)
	if sum.PerMinute > 0 {
		fmt.Fprintf(&b, "   %.1f/min", sum.PerMinute)
	}
	fmt.Fprintf(&b, "\n2xx %.1f%%   3xx %.1f%%   4xx %.1f%%   5xx %.1f%%\n\n",
		pct(sum.ByClass["2xx"]), pct(sum.ByClass["3xx"]), pct(sum.ByClass["4xx"]), pct(sum.ByClass["5xx"]))
	section := func(title string, counts []logs.Count) {
		b.WriteString(headerStyle.Render(title) + "\n")
		if len(counts) == 0 {
			b.WriteString(mutedStyle.Render("  none") + "\n")
		}
		for _, c := range counts {
			fmt.Fprintf(&b, "  %6d  %s\n", c.N, c.Value)
		}
		b.WriteString("\n")
	}
	section("Top clients", sum.TopIPs)
	section("Top paths", sum.TopPaths)
	section("Paths answered with 5xx", sum.Top5xx)
	return b.String()
}
