package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// frameChrome is how many terminal rows renderFrame uses around the body
// (title bar, borders, help line).
const frameChrome = 4

// ViewFit renders the menu below above (the body text already rendered on
// the screen) within a terminal of the given height, scrolling to keep the
// cursor visible when the items do not all fit.
func (m *simpleMenu) ViewFit(above string, height int) string {
	rows := height - frameChrome - lipgloss.Height(above)
	if above == "" {
		rows++
	}
	if rows >= len(m.items) || rows < 3 {
		return m.View()
	}
	rows -= 2 // the "more" indicators
	start := m.cursor - rows/2
	if start < 0 {
		start = 0
	}
	if start+rows > len(m.items) {
		start = len(m.items) - rows
	}
	var b strings.Builder
	if start > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	} else {
		b.WriteString("\n")
	}
	window := &simpleMenu{items: m.items[start : start+rows], cursor: m.cursor - start}
	b.WriteString(window.View())
	if rest := len(m.items) - start - rows; rest > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ↓ %d more", rest)) + "\n")
	}
	return b.String()
}
