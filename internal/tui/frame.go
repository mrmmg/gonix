package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// instanceLabel names the Nginx instance being managed, shown in the title
// bar; empty for the default instance.
var instanceLabel string

// renderFrame draws the standard application chrome (title bar with version,
// bordered content area, help footer) around body, matching the look
// described in the project's UI/UX specification.
func renderFrame(width, height int, title string, body string, help [][2]string) string {
	if width < 20 {
		width = 20
	}
	if height < 10 {
		height = 10
	}

	if instanceLabel != "" {
		title += " · " + instanceLabel
	}
	titleBar := titleBarStyle.Render(title)
	version := versionStyle.Render("v" + Version)
	gap := width - lipgloss.Width(titleBar) - lipgloss.Width(version) - 2
	if gap < 1 {
		gap = 1
	}
	bar := titleBar + strings.Repeat(" ", gap) + version

	helpLine := renderHelp(help)

	// The box fills every row the title bar, its two borders and the help
	// line leave. Height is only a minimum in Lip Gloss, so MaxHeight cuts
	// a body that is too long instead of letting it push the frame past
	// the bottom of the terminal.
	contentWidth := width - 4
	contentHeight := height - 3 - lipgloss.Height(helpLine)
	if contentWidth < 10 {
		contentWidth = 10
	}
	if contentHeight < 3 {
		contentHeight = 3
	}

	content := lipgloss.NewStyle().Width(contentWidth).Height(contentHeight).MaxHeight(contentHeight).Render(body)

	boxed := appBorderStyle.Width(width - 2).Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, bar, boxed, helpLine)
}
