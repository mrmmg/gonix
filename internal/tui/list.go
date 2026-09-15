package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// menuItem is a single selectable row in a simpleMenu.
type menuItem struct {
	title string
	desc  string // rendered dimmed to the right of title, optional
}

// simpleMenu is a minimal, consistently styled vertical menu shared by every
// screen in the application (main menu, host submenu, settings, ...). It
// intentionally avoids bubbles/list so the visual style stays uniform and
// simple across the whole app.
type simpleMenu struct {
	items  []menuItem
	cursor int
}

func newSimpleMenu(items []menuItem) *simpleMenu {
	return &simpleMenu{items: items}
}

// HandleKey processes a navigation key press and returns true if it moved
// the cursor or otherwise consumed the key.
func (m *simpleMenu) HandleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return true
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		return true
	}
	return false
}

func (m *simpleMenu) Selected() int { return m.cursor }

func (m *simpleMenu) SetItems(items []menuItem) {
	m.items = items
	if m.cursor >= len(items) {
		m.cursor = len(items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *simpleMenu) View() string {
	var b strings.Builder
	for i, item := range m.items {
		cursor := "  "
		style := normalItemStyle
		if i == m.cursor {
			cursor = "▸ "
			style = selectedItemStyle
		}
		line := cursor + item.title
		if item.desc != "" {
			line += "  " + mutedStyle.Render(item.desc)
		}
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
	return b.String()
}
