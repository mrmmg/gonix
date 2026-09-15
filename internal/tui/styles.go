// Package tui implements the interactive terminal UI for GoNix,
// built on Bubble Tea/Bubbles/Lip Gloss. It never touches Nginx
// configuration files directly; all mutations go through internal/hosts,
// internal/accesslist, internal/certificates and internal/system.
package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.Color("39")  // blue
	colorAccent    = lipgloss.Color("212") // pink
	colorSuccess   = lipgloss.Color("42")  // green
	colorWarning   = lipgloss.Color("214") // orange
	colorDanger    = lipgloss.Color("196") // red
	colorMuted     = lipgloss.Color("240") // gray
	colorSubtle    = lipgloss.Color("245")
	colorTextOnBar = lipgloss.Color("230")

	titleBarStyle = lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(colorTextOnBar).
			Bold(true).
			Padding(0, 2)

	versionStyle = lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(colorTextOnBar).
			Padding(0, 2)

	appBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)

	successStyle = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(colorWarning).Bold(true)
	dangerStyle  = lipgloss.NewStyle().Foreground(colorDanger).Bold(true)

	enabledDot  = successStyle.Render("●")
	disabledDot = mutedStyle.Render("○")

	helpKeyStyle  = lipgloss.NewStyle().Foreground(colorAccent)
	helpDescStyle = lipgloss.NewStyle().Foreground(colorSubtle)

	errorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDanger).
			Foreground(colorDanger).
			Padding(0, 1)

	successBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSuccess).
			Foreground(colorSuccess).
			Padding(0, 1)

	inputLabelStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)

	focusedInputStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(0, 1)

	blurredInputStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(colorMuted).
				Padding(0, 1)
)

// renderHelp formats a list of (key, description) pairs into the standard
// status bar help line, e.g. "↑↓ Navigate    Enter Select    q Quit".
func renderHelp(pairs [][2]string) string {
	out := ""
	for i, p := range pairs {
		if i > 0 {
			out += "    "
		}
		out += helpKeyStyle.Render(p[0]) + " " + helpDescStyle.Render(p[1])
	}
	return statusBarStyle.Render(out)
}
