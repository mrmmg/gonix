package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestViewerUsesTheWholeHeight(t *testing.T) {
	var lines []string
	for i := 1; i <= 200; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	v := newViewerScreen("T", strings.Join(lines, "\n"), nil)
	for _, height := range []int{20, 50} {
		out := v.View(100, height)
		rows := height - frameChrome - lipgloss.Height(headerStyle.Render("T")+"\n\n")
		if !strings.Contains(out, fmt.Sprintf("line %d\n", rows)) && !strings.Contains(out, fmt.Sprintf("line %d ", rows)) {
			t.Errorf("height %d: line %d not shown", height, rows)
		}
		if strings.Contains(out, fmt.Sprintf("line %d ", rows+1)) {
			t.Errorf("height %d: more lines than fit", height)
		}
		if !strings.Contains(out, fmt.Sprintf("lines 1–%d of 200 · ↓ %d more", rows, 200-rows)) {
			t.Errorf("height %d: no position line", height)
		}
		if got := lipgloss.Height(out); got != height {
			t.Errorf("height %d: rendered %d rows", height, got)
		}
	}
	// Paging and End stop at a full last page.
	v.View(100, 50)
	rows := v.rows
	v.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if out := v.View(100, 50); !strings.Contains(out, fmt.Sprintf("lines %d–200 of 200", 200-rows+1)) {
		t.Errorf("End:\n%s", out)
	}
	v.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if out := v.View(100, 50); !strings.Contains(out, fmt.Sprintf("lines %d–%d of 200", 200-2*rows+1, 200-rows)) {
		t.Errorf("PgUp:\n%s", out)
	}
	// Short text: no scrolling hints.
	if out := newViewerScreen("T", "a\nb", nil).View(100, 50); strings.Contains(out, "lines ") || strings.Contains(out, "PgUp") {
		t.Errorf("short text shows scrolling:\n%s", out)
	}
	// Long lines are cut, not wrapped into the next rows.
	if out := newViewerScreen("T", strings.Repeat("x", 300), nil).View(80, 30); lipgloss.Height(out) != 30 {
		t.Errorf("a long line changed the layout")
	}
}
