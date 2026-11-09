package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/update"
)

// newUpdateClient is replaced by tests; the real one only talks to GitHub.
var newUpdateClient = update.NewClient

// updateCheckedMsg is the result of a check that ran in the background.
type updateCheckedMsg struct {
	rel update.Release
	err error
}

// checkUpdateCmd asks GitHub (or reuses a check from the last 24 hours,
// unless force is set).
func checkUpdateCmd(deps Deps, force bool) tea.Cmd {
	cache := update.CachePath(deps.Config.Logs.AuditFile)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(backgroundCtx(), 20*time.Second)
		defer cancel()
		rel, err := newUpdateClient().Check(ctx, cache, force)
		return updateCheckedMsg{rel: rel, err: err}
	}
}

// knownRelease is the release found by the last check, if it is newer than
// the running version.
func knownRelease(deps Deps) (update.Release, bool) {
	rel, _, ok := update.ReadCache(update.CachePath(deps.Config.Logs.AuditFile))
	return rel, ok && update.Newer(rel.Tag, Version)
}

// notesLines is the start of a release's notes, as lines for a callout.
func notesLines(notes string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(notes), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
		if len(out) == 8 {
			out = append(out, "…")
			break
		}
	}
	return out
}

const howToUpdate = `GoNix updates itself from the command line, not from this menu: replacing a program that is running is safer done by a command that finishes by itself.

  sudo gonix update              ask, then update
  sudo gonix update --yes        update without asking
  gonix update --check           only say whether a newer release exists
  sudo gonix update --rollback   put the previous version back

What it does:
  1. reads the latest release from github.com/mrmmg/gonix (HTTPS only)
  2. downloads the binary for this machine and its SHA-256 checksum
  3. refuses the update if the checksum does not match
  4. checks that the new binary runs
  5. keeps the current version as gonix.old (always one previous version)
  6. replaces gonix, and restarts the monitoring exporter if it is running
     (Settings: "Restart the monitoring exporter after an update")

Your configuration and hosts are not touched. Run gonix again afterwards to use the new version.

Updates come only from GitHub Releases. The daily check that shows the notice in the main menu can be turned off in Settings ("Check for updates").`

// updateScreen is the Updates menu: what is installed, whether a newer
// release exists, and how to update.
type updateScreen struct {
	deps     Deps
	menu     *simpleMenu
	rel      *update.Release
	err      string
	checking bool
}

const (
	uaCheck = iota
	uaHow
)

func newUpdateScreen(deps Deps) *updateScreen {
	s := &updateScreen{deps: deps}
	if rel, _, ok := update.ReadCache(update.CachePath(deps.Config.Logs.AuditFile)); ok {
		s.rel = &rel
	}
	s.menu = newSimpleMenu([]menuItem{
		{title: "Check Now", desc: "ask GitHub for the latest release"},
		{title: "How to Update", desc: "the command, and what it does"},
	})
	return s
}

func (s *updateScreen) Init() tea.Cmd {
	if s.deps.Config.Update.Check && (s.rel == nil || !s.fresh()) {
		s.checking = true
		return checkUpdateCmd(s.deps, false)
	}
	return nil
}

func (s *updateScreen) fresh() bool {
	_, fresh, _ := update.ReadCache(update.CachePath(s.deps.Config.Logs.AuditFile))
	return fresh
}

func (s *updateScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case updateCheckedMsg:
		s.checking = false
		if msg.err != nil {
			s.err = msg.err.Error()
			return s, nil
		}
		s.err, s.rel = "", &msg.rel
	case tea.KeyMsg:
		if s.menu.HandleKey(msg) {
			return s, nil
		}
		switch msg.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			switch s.menu.Selected() {
			case uaCheck:
				s.checking, s.err = true, ""
				return s, checkUpdateCmd(s.deps, true)
			case uaHow:
				return newViewerScreen("How to Update GoNix", howToUpdate, s), nil
			}
		}
	}
	return s, nil
}

func (s *updateScreen) View(width, height int) string {
	body := headerStyle.Render("Updates") + "\n"
	body += "Installed version: v" + Version + "\n"
	switch {
	case s.checking:
		body += mutedStyle.Render("Checking GitHub…") + "\n"
	case s.err != "":
		body += callout(width-4, calloutWarning, "Could not check for updates", []string{s.err}) + "\n"
	case s.rel != nil && update.Newer(s.rel.Tag, Version):
		lines := append([]string{"Update from a terminal:   sudo gonix update", "Release page: " + s.rel.URL, ""}, notesLines(s.rel.Notes)...)
		body += callout(width-4, calloutInfo, "Version "+s.rel.Version+" is available", lines) + "\n"
	case s.rel != nil:
		body += successStyle.Render("✓ GoNix is up to date") + mutedStyle.Render(" (latest release: "+s.rel.Version+")") + "\n"
	case !s.deps.Config.Update.Check:
		body += mutedStyle.Render("Automatic update checks are off (Settings). Use Check Now to look.") + "\n"
	}
	body += "\n" + s.menu.ViewFit(body, height)
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", body, help)
}
