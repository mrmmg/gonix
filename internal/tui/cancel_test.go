package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Esc in the Add New Host wizard cancels it. Its cancel hook returns no
// screen of its own, only "pop me"; the root must keep the wizard on top
// until that pop is handled, or the next View dereferences nil and the whole
// program dies with a panic.
func TestCancelAddHostWizardReturnsToPreviousScreen(t *testing.T) {
	deps := testDeps(t)
	root := NewApp(deps).(*rootModel)
	root.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	root.Update(navPushMsg{screen: newAddHostWizard(deps)})
	if len(root.stack) != 2 {
		t.Fatalf("stack = %d screens, want 2", len(root.stack))
	}

	_, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if root.top() == nil {
		t.Fatal("Esc left a nil screen on top of the stack")
	}
	_ = root.View() // must not panic

	if cmd == nil {
		t.Fatal("Esc should ask to leave the wizard")
	}
	root.Update(cmd())
	if len(root.stack) != 1 {
		t.Errorf("stack = %d screens after Esc, want 1 (back at the main menu)", len(root.stack))
	}
	if _, ok := root.top().(*mainMenu); !ok {
		t.Errorf("top = %T, want the main menu", root.top())
	}
	_ = root.View()
}
