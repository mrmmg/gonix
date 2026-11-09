package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// calloutKind picks the colours of a callout.
type calloutKind int

const (
	calloutDanger calloutKind = iota
	calloutWarning
	calloutInfo
)

// callout draws a notice as a block with a coloured bar on its left, a bold
// title and one line per item, so it stands out from the menu below it
// instead of being one long line of red text. width is the width available.
func callout(width int, kind calloutKind, title string, lines []string) string {
	color, bg, icon := colorDanger, lipgloss.Color("52"), "⚠ "
	switch kind {
	case calloutWarning:
		color, bg = colorWarning, lipgloss.Color("94")
	case calloutInfo:
		color, bg, icon = colorPrimary, lipgloss.Color("24"), "ⓘ "
	}
	if width < 24 {
		width = 24
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(color).
		Background(bg).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Width(width - 1)
	head := lipgloss.NewStyle().Bold(true).Foreground(color).Background(bg).Render(icon + title)
	body := head
	if len(lines) > 0 {
		body += "\n" + strings.Join(lines, "\n")
	}
	return box.Render(body) + "\n"
}
