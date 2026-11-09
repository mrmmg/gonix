package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// menuItem is a single selectable row in a simpleMenu.
type menuItem struct {
	title string
	desc  string // rendered dimmed to the right of title, optional
	// search is extra text a search matches besides title and desc, e.g.
	// a host's aliases.
	search string
	// pinned rows (such as "+ New ...") stay in the list while searching.
	pinned bool
}

// simpleMenu is a minimal, consistently styled vertical menu shared by every
// screen in the application (main menu, host submenu, settings, ...). It
// intentionally avoids bubbles/list so the visual style stays uniform and
// simple across the whole app.
//
// A menu made with newSearchMenu can be filtered: Ctrl+F opens a search box,
// and Selected() keeps returning the index into the items the menu was
// given, whatever is filtered out (or -1 when nothing matches).
type simpleMenu struct {
	items  []menuItem
	cursor int // position among the rows shown

	searchable bool
	searching  bool
	hint       string // what a search matches, shown as the placeholder
	input      textinput.Model
	filtered   []menuItem  // rows matching the query, best first, while filtering
	origin     []int       // index in items of each filtered row
	matches    []itemMatch // what to highlight in each filtered row
}

func newSimpleMenu(items []menuItem) *simpleMenu {
	return &simpleMenu{items: items}
}

// newSearchMenu is a menu that Ctrl+F can search; hint says what a search
// matches, e.g. "domain, alias or backend".
func newSearchMenu(items []menuItem, hint string) *simpleMenu {
	m := &simpleMenu{items: items, searchable: true, hint: hint}
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = hint
	ti.Width = 36
	m.input = ti
	return m
}

func (m *simpleMenu) filtering() bool {
	return m.searching && strings.TrimSpace(m.input.Value()) != ""
}

// refilter recomputes the rows shown for the current query: the rows that
// match every word of it (see fuzzyMatch), best match first; pinned rows
// stay on top.
func (m *simpleMenu) refilter() {
	m.filtered, m.origin, m.matches = nil, nil, nil
	if !m.filtering() {
		return
	}
	q := m.input.Value()
	all := make([]itemMatch, len(m.items))
	var pinned, found []int
	for i, it := range m.items {
		if it.pinned {
			pinned = append(pinned, i)
			continue
		}
		if im, ok := matchItem(q, it); ok {
			all[i] = im
			found = append(found, i)
		}
	}
	rank(found, all)
	for _, i := range append(pinned, found...) {
		m.filtered = append(m.filtered, m.items[i])
		m.origin = append(m.origin, i)
		m.matches = append(m.matches, all[i])
	}
}

// visible is the rows currently shown.
func (m *simpleMenu) visible() []menuItem {
	if m.filtering() {
		return m.filtered
	}
	return m.items
}

// Searching reports whether the search box is open.
func (m *simpleMenu) Searching() bool { return m.searching }

func (m *simpleMenu) startSearch() {
	m.searching = true
	m.input.SetValue("")
	m.input.Focus()
}

func (m *simpleMenu) stopSearch() {
	sel := m.Selected()
	m.searching = false
	m.input.SetValue("")
	m.input.Blur()
	m.refilter()
	// Keep the cursor on the row that was selected.
	if sel >= 0 {
		m.cursor = sel
	}
	m.clamp()
}

func (m *simpleMenu) clamp() {
	if n := len(m.visible()); m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// HandleKey processes a navigation key press and returns true if it moved
// the cursor or otherwise consumed the key. While the search box is open it
// consumes everything except Enter, so typing never triggers a shortcut.
func (m *simpleMenu) HandleKey(msg tea.KeyMsg) bool {
	key := msg.String()
	if m.searchable {
		switch {
		case !m.searching && key == "ctrl+f":
			m.startSearch()
			return true
		case m.searching:
			switch key {
			case "esc":
				m.stopSearch()
			case "enter":
				return false
			case "up", "down":
				m.move(key == "down")
			case "ctrl+f":
			default:
				before := m.input.Value()
				m.input, _ = m.input.Update(msg)
				if m.input.Value() != before {
					m.refilter()
					m.cursor = 0
					for i, it := range m.visible() {
						if !it.pinned {
							m.cursor = i
							break
						}
					}
				}
			}
			return true
		}
	}
	switch key {
	case "up", "k":
		m.move(false)
		return true
	case "down", "j":
		m.move(true)
		return true
	}
	return false
}

func (m *simpleMenu) move(down bool) {
	switch {
	case down && m.cursor < len(m.visible())-1:
		m.cursor++
	case !down && m.cursor > 0:
		m.cursor--
	}
}

// Selected is the index, in the items the menu was given, of the selected
// row; -1 when a search matches nothing.
func (m *simpleMenu) Selected() int {
	if !m.filtering() {
		return m.cursor
	}
	if m.cursor < len(m.origin) {
		return m.origin[m.cursor]
	}
	return -1
}

func (m *simpleMenu) SetItems(items []menuItem) {
	m.items = items
	m.refilter()
	m.clamp()
}

// searchBar is the box above the rows of a searchable menu: a dim box that
// names the key while closed, a highlighted one with the text typed so far
// while open.
func (m *simpleMenu) searchBar() string {
	if !m.searchable {
		return ""
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(60)
	if !m.searching {
		return box.BorderForeground(colorMuted).Render(mutedStyle.Render("Ctrl + F to search ("+m.hint+")")) + "\n"
	}
	count := ""
	if m.filtering() {
		count = mutedStyle.Render(fmt.Sprintf("  %d of %d", len(m.origin)-m.pinnedCount(), len(m.items)-m.pinnedCount()))
	}
	return box.BorderForeground(colorAccent).Render(inputLabelStyle.Render("Search ▸ ")+m.input.View()+count) + "\n"
}

func (m *simpleMenu) pinnedCount() int {
	n := 0
	for _, it := range m.items {
		if it.pinned {
			n++
		}
	}
	return n
}

func renderRows(items []menuItem, cursor int, hl []itemMatch) string {
	lines := make([]string, len(items))
	plain := make([]string, len(items))
	width := 0
	for i, item := range items {
		title, desc := item.title, item.desc
		if i < len(hl) {
			title, desc = highlight(title, hl[i].title), highlight(desc, hl[i].desc)
		}
		line, bare := "  "+title, "  "+item.title
		if desc != "" {
			if i == cursor {
				line += "  " + desc
			} else {
				line += "  " + mutedStyle.Render(desc)
			}
			bare += "  " + item.desc
		}
		lines[i], plain[i] = line, bare
		width = max(width, lipgloss.Width(bare))
	}
	var b strings.Builder
	for i, line := range lines {
		if i == cursor {
			b.WriteString(renderSelected(line, width+1))
		} else {
			b.WriteString(normalItemStyle.Render(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m *simpleMenu) View() string {
	rows := m.visible()
	body := renderRows(rows, m.cursor, m.matches)
	if m.filtering() && len(m.origin) == m.pinnedCount() {
		body += mutedStyle.Render("  (no matches)") + "\n"
	}
	return m.searchBar() + body
}

// searchHelp is the help line of a screen with a menu: pairs, plus the
// search key; while the search box is open, the keys that work in it.
func searchHelp(m *simpleMenu, pairs [][2]string) [][2]string {
	if !m.searchable {
		return pairs
	}
	if m.searching {
		return [][2]string{{"type", "Filter"}, {"↑↓", "Navigate"}, {"Enter", "Open"}, {"Esc", "Clear search"}}
	}
	out := make([][2]string, 0, len(pairs)+1)
	for _, p := range pairs {
		if p[0] == "Esc" || p[0] == "q" {
			out = append(out, [2]string{"Ctrl+F", "Search"})
		}
		out = append(out, p)
	}
	return out
}

// frameChrome is how many terminal rows renderFrame uses around the body
// (title bar, borders, help line).
const frameChrome = 4

// ViewFit renders the menu below above (the body text already rendered on
// the screen) within a terminal of the given height, scrolling to keep the
// cursor visible when the items do not all fit.
func (m *simpleMenu) ViewFit(above string, height int) string {
	bar := m.searchBar()
	items := m.visible()
	rows := height - frameChrome - lipgloss.Height(above) - strings.Count(bar, "\n")
	if above == "" {
		rows++
	}
	if m.filtering() && len(items) == m.pinnedCount() {
		rows--
	}
	if rows >= len(items) || rows < 3 {
		return m.View()
	}
	rows -= 2 // the "more" indicators
	start := m.cursor - rows/2
	if start < 0 {
		start = 0
	}
	if start+rows > len(items) {
		start = len(items) - rows
	}
	var b strings.Builder
	b.WriteString(bar)
	if start > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	} else {
		b.WriteString("\n")
	}
	var hl []itemMatch
	if m.filtering() {
		hl = m.matches[start : start+rows]
	}
	b.WriteString(renderRows(items[start:start+rows], m.cursor-start, hl))
	if rest := len(items) - start - rows; rest > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ↓ %d more", rest)) + "\n")
	}
	return b.String()
}
