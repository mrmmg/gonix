package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// confirmScreen asks a Yes/No question before a destructive or otherwise
// significant action.
type confirmScreen struct {
	title   string
	message string
	cursor  int // 0 = No, 1 = Yes
	onYes   func() (screen, tea.Cmd)
	onNo    func() (screen, tea.Cmd)
}

func newConfirmScreen(title, message string, onYes, onNo func() (screen, tea.Cmd)) *confirmScreen {
	return &confirmScreen{title: title, message: message, onYes: onYes, onNo: onNo}
}

func (s *confirmScreen) Init() tea.Cmd { return nil }

func (s *confirmScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "left", "h", "up", "k", "right", "l", "down", "j", " ":
			s.cursor = 1 - s.cursor
		case "y":
			return s.onYes()
		case "n", "esc":
			return s.onNo()
		case "enter":
			if s.cursor == 1 {
				return s.onYes()
			}
			return s.onNo()
		}
	}
	return s, nil
}

func (s *confirmScreen) View(width, height int) string {
	body := headerStyle.Render(s.title) + "\n\n" + s.message + "\n\n"
	no, yes := "  No", "  Yes"
	if s.cursor == 0 {
		no = selectedItemStyle.Render("▸ No")
	} else {
		yes = selectedItemStyle.Render("▸ Yes")
	}
	body += no + "    " + yes
	help := [][2]string{{"y/n", "Confirm"}, {"Enter", "Select"}, {"Esc", "Cancel"}}
	return renderFrame(width, height, "GONIX", body, help)
}
