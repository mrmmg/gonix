package tui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shiva/nginx-manager/internal/backup"
	"github.com/shiva/nginx-manager/internal/nginx"
)

// backupScreen lists stored configuration snapshots for a host and lets the
// operator restore one (validated and rolled back like any other change).
type backupScreen struct {
	deps  Deps
	host  nginx.ListedHost
	snaps []backup.Snapshot
	menu  *simpleMenu
	err   string
}

func newBackupScreen(deps Deps, h nginx.ListedHost) *backupScreen {
	s := &backupScreen{deps: deps, host: h}
	s.reload()
	return s
}

func (s *backupScreen) reload() {
	snaps, err := s.deps.Backups.List(s.host.FileName)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.snaps = snaps
	items := make([]menuItem, 0, len(snaps))
	for _, snap := range snaps {
		items = append(items, menuItem{title: snap.Timestamp.Format("2006-01-02 15:04:05"), desc: filepath.Base(snap.Path)})
	}
	if len(items) == 0 {
		items = append(items, menuItem{title: "(no backups yet)"})
	}
	s.menu = newSimpleMenu(items)
}

func (s *backupScreen) Init() tea.Cmd { return nil }

func (s *backupScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			if len(s.snaps) == 0 {
				return s, nil
			}
			snap := s.snaps[s.menu.Selected()]
			return newConfirmScreen(
				"Restore Backup",
				fmt.Sprintf("Restore %s from %s? The current configuration will itself be backed up first, and restored automatically if the result fails validation.",
					s.host.ServerName, snap.Timestamp.Format("2006-01-02 15:04:05")),
				func() (screen, tea.Cmd) { return s.restore(snap) },
				func() (screen, tea.Cmd) { return s, nil },
			), nil
		}
	}
	return s, nil
}

func (s *backupScreen) restore(snap backup.Snapshot) (screen, tea.Cmd) {
	availablePath := filepath.Join(s.deps.Manager.SitesAvailable, s.host.FileName)
	if _, err := s.deps.Backups.Save(s.host.FileName, availablePath); err != nil {
		return newResultScreen(s.deps, "Restore Backup", false, err.Error(), s), nil
	}
	if err := s.deps.Backups.Restore(snap, availablePath); err != nil {
		return newResultScreen(s.deps, "Restore Backup", false, err.Error(), s), nil
	}
	result, err := s.deps.HostService.Tester.Test(backgroundCtx())
	if err != nil || !result.OK {
		msg := "configuration test failed after restore"
		if err == nil {
			msg = result.Output
		}
		return newResultScreen(s.deps, "Restore Backup", false, msg, s), nil
	}
	if s.deps.HostService.Reloader != nil {
		_ = s.deps.HostService.Reloader.Reload(backgroundCtx())
	}
	return newResultScreen(s.deps, "Restore Backup", true, "Backup restored successfully.", newHostDetailScreen(s.deps, s.host)), nil
}

func (s *backupScreen) View(width, height int) string {
	body := headerStyle.Render("Backup / Restore — "+s.host.ServerName) + "\n\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.View()
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Restore"}, {"Esc", "Back"}}
	return renderFrame(width, height, "NGINX MANAGER", body, help)
}
