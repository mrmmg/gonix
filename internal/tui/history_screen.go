package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/history"
)

// historyScreen lists configuration commits; Enter shows a commit's diff
// and r restores the configuration to it.
type historyScreen struct {
	deps    Deps
	commits []history.Commit
	menu    *simpleMenu
	err     string
}

func newHistoryScreen(deps Deps) *historyScreen {
	s := &historyScreen{deps: deps, menu: newSearchMenu(nil, "message, date or commit")}
	if deps.History == nil {
		s.err = "Configuration history is disabled. Install git and set history.enabled: true in /etc/gonix/gonix.yaml."
		return s
	}
	commits, err := deps.History.Log(backgroundCtx(), 200)
	if err != nil {
		s.err = err.Error()
	}
	s.commits = commits
	items := make([]menuItem, 0, len(commits))
	for _, c := range commits {
		items = append(items, menuItem{title: c.Time.Format("2006-01-02 15:04:05") + "  " + c.Hash[:8], desc: c.Message})
	}
	if len(items) == 0 {
		items = append(items, menuItem{title: "(no history yet)"})
	}
	s.menu.SetItems(items)
	return s
}

func (s *historyScreen) Init() tea.Cmd { return nil }

func (s *historyScreen) selected() (history.Commit, bool) {
	i := s.menu.Selected()
	if len(s.commits) == 0 || i < 0 || i >= len(s.commits) {
		return history.Commit{}, false
	}
	return s.commits[i], true
}

func (s *historyScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
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
		c, ok := s.selected()
		if !ok {
			return s, nil
		}
		diff, err := s.deps.History.Show(backgroundCtx(), c.Hash)
		if err != nil {
			return newResultScreen(s.deps, "History", false, err.Error(), s), nil
		}
		return newViewerScreen("Commit "+c.Hash[:8], diff, s), nil
	case "r":
		c, ok := s.selected()
		if !ok {
			return s, nil
		}
		return newConfirmScreen("Restore Configuration",
			fmt.Sprintf("Restore the Nginx configuration to %s (%s)?\n\nThe current state is committed first, so this can be undone.",
				c.Time.Format("2006-01-02 15:04:05"), c.Message),
			func() (screen, tea.Cmd) { return s.restore(c) },
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return s, nil
}

func (s *historyScreen) restore(c history.Commit) (screen, tea.Cmd) {
	ctx := backgroundCtx()
	err := s.deps.History.Restore(ctx, c.Hash, s.deps.HostService.Tester)
	if err == nil && s.deps.HostService.Reloader != nil {
		err = s.deps.HostService.Reloader.Reload(ctx)
	}
	result := audit.ResultSuccess
	if err != nil {
		result = audit.ResultFailure
	}
	if s.deps.Audit != nil {
		e := audit.Entry{User: audit.CurrentUser(), Action: "history_restored", Target: c.Hash[:10], Result: result}
		if err != nil {
			e.Error = err.Error()
		}
		_ = s.deps.Audit.Log(e)
	}
	if err != nil {
		return newResultScreen(s.deps, "Restore Configuration", false, err.Error(), newHistoryScreen(s.deps)), nil
	}
	return newResultScreen(s.deps, "Restore Configuration", true,
		"Configuration restored to "+c.Hash[:8]+" and Nginx reloaded.", newHistoryScreen(s.deps)), nil
}

func (s *historyScreen) View(width, height int) string {
	body := headerStyle.Render("Configuration History") + "\n"
	body += mutedStyle.Render("Every change made through GoNix is committed; restore any earlier state.") + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.ViewFit(body, height)
	}
	help := searchHelp(s.menu, [][2]string{{"↑↓", "Navigate"}, {"Enter", "Show diff"}, {"r", "Restore"}, {"Esc", "Back"}})
	return renderFrame(width, height, "GONIX", body, help)
}
