package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

	titleBar := titleBarStyle.Render(title)
	version := versionStyle.Render("v" + Version)
	gap := width - lipgloss.Width(titleBar) - lipgloss.Width(version) - 2
	if gap < 1 {
		gap = 1
	}
	bar := titleBar + strings.Repeat(" ", gap) + version

	contentWidth := width - 4
	contentHeight := height - 6
	if contentWidth < 10 {
		contentWidth = 10
	}
	if contentHeight < 3 {
		contentHeight = 3
	}

	content := lipgloss.NewStyle().Width(contentWidth).Height(contentHeight).Render(body)
	helpLine := renderHelp(help)

	boxed := appBorderStyle.Width(width - 2).Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, bar, boxed, helpLine)
}
