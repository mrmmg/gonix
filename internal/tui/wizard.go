package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// fieldKind identifies how a wizardField is presented and edited.
type fieldKind int

const (
	fieldText fieldKind = iota
	fieldBool
	fieldChoice
)

// wizardField describes a single question in a sequential wizard, e.g. "Add
// New Host". Wizards intentionally ask one question at a time (rather than
// a multi-field grid) to keep the flow simple and closely mirror how a human
// operator would be interviewed for the required information.
type wizardField struct {
	Key         string
	Label       string
	Kind        fieldKind
	Placeholder string
	Default     string   // for fieldText
	Sensitive   bool     // for fieldText: mask input, e.g. API tokens
	BoolDefault bool     // for fieldBool
	Options     []string // for fieldChoice
	Validate    func(value string) error
	// ShowIf allows a field to be skipped based on previously collected
	// answers, e.g. only asking for the upstream port in Reverse Proxy mode.
	ShowIf func(values map[string]string) bool
}

// wizardScreen drives a sequence of wizardFields, collecting answers into a
// map keyed by field.Key, then invokes onDone.
type wizardScreen struct {
	title  string
	fields []wizardField
	idx    int
	values map[string]string

	input      textinput.Model
	boolCursor int // 0 = No, 1 = Yes
	choiceIdx  int
	errMsg     string

	onDone   func(values map[string]string) (screen, tea.Cmd)
	onCancel func() (screen, tea.Cmd)
}

func newWizard(title string, fields []wizardField, onDone func(map[string]string) (screen, tea.Cmd), onCancel func() (screen, tea.Cmd)) *wizardScreen {
	w := &wizardScreen{title: title, fields: fields, values: map[string]string{}, onDone: onDone, onCancel: onCancel}
	w.enterStep(0)
	return w
}

// enterStep positions the wizard at index i, skipping any fields whose
// ShowIf returns false, and prepares the appropriate input widget.
func (w *wizardScreen) enterStep(i int) {
	for i < len(w.fields) {
		f := w.fields[i]
		if f.ShowIf != nil && !f.ShowIf(w.values) {
			i++
			continue
		}
		break
	}
	w.idx = i
	w.errMsg = ""
	if i >= len(w.fields) {
		return
	}
	f := w.fields[i]
	switch f.Kind {
	case fieldText:
		ti := textinput.New()
		ti.Placeholder = f.Placeholder
		ti.SetValue(f.Default)
		ti.CursorEnd()
		ti.Focus()
		ti.Width = 50
		if f.Sensitive {
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
		}
		w.input = ti
	case fieldBool:
		w.boolCursor = 0
		if f.BoolDefault {
			w.boolCursor = 1
		}
	case fieldChoice:
		w.choiceIdx = 0
	}
}

func (w *wizardScreen) currentField() (wizardField, bool) {
	if w.idx >= len(w.fields) {
		return wizardField{}, false
	}
	return w.fields[w.idx], true
}

func (w *wizardScreen) Init() tea.Cmd { return textinput.Blink }

func (w *wizardScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	f, ok := w.currentField()
	if !ok {
		return w, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if w.onCancel != nil {
				return w.onCancel()
			}
			return w, navPop()
		}

		switch f.Kind {
		case fieldText:
			switch msg.String() {
			case "enter":
				val := w.input.Value()
				if f.Validate != nil {
					if err := f.Validate(val); err != nil {
						w.errMsg = err.Error()
						return w, nil
					}
				}
				w.values[f.Key] = val
				w.enterStep(w.idx + 1)
				if _, more := w.currentField(); !more {
					return w.finish()
				}
				return w, textinput.Blink
			default:
				var cmd tea.Cmd
				w.input, cmd = w.input.Update(msg)
				return w, cmd
			}

		case fieldBool:
			switch msg.String() {
			case "left", "h", "up", "k":
				w.boolCursor = 0
			case "right", "l", "down", "j":
				w.boolCursor = 1
			case " ":
				w.boolCursor = 1 - w.boolCursor
			case "enter":
				val := "no"
				if w.boolCursor == 1 {
					val = "yes"
				}
				w.values[f.Key] = val
				w.enterStep(w.idx + 1)
				if _, more := w.currentField(); !more {
					return w.finish()
				}
				return w, textinput.Blink
			}

		case fieldChoice:
			switch msg.String() {
			case "up", "k":
				if w.choiceIdx > 0 {
					w.choiceIdx--
				}
			case "down", "j":
				if w.choiceIdx < len(f.Options)-1 {
					w.choiceIdx++
				}
			case "enter":
				w.values[f.Key] = f.Options[w.choiceIdx]
				w.enterStep(w.idx + 1)
				if _, more := w.currentField(); !more {
					return w.finish()
				}
				return w, textinput.Blink
			}
		}
	}
	return w, nil
}

// finish hands off to onDone once every field has been answered. It routes
// the resulting screen through navReplace rather than swapping it in
// directly, because only the navReplaceMsg path calls the new screen's
// Init() — which matters for anything beyond static screens, e.g.
// liveRunScreen relies on Init() to start listening for its background
// run's progress messages at all.
func (w *wizardScreen) finish() (screen, tea.Cmd) {
	next, cmd := w.onDone(w.values)
	return w, tea.Batch(cmd, navReplace(next))
}

func (w *wizardScreen) View(width, height int) string {
	f, ok := w.currentField()
	if !ok {
		return "Working..."
	}

	body := headerStyle.Render(w.title) + "\n\n"
	body += inputLabelStyle.Render(f.Label) + "\n\n"

	switch f.Kind {
	case fieldText:
		body += focusedInputStyle.Render(w.input.View())
	case fieldBool:
		no, yes := "No", "Yes"
		if w.boolCursor == 0 {
			no = selectedItemStyle.Render("▸ No")
		} else {
			no = normalItemStyle.Render("  No")
		}
		if w.boolCursor == 1 {
			yes = selectedItemStyle.Render("▸ Yes")
		} else {
			yes = normalItemStyle.Render("  Yes")
		}
		body += no + "    " + yes
	case fieldChoice:
		for i, opt := range f.Options {
			if i == w.choiceIdx {
				body += selectedItemStyle.Render("▸ "+opt) + "\n"
			} else {
				body += normalItemStyle.Render("  "+opt) + "\n"
			}
		}
	}

	body += "\n\n" + mutedStyle.Render(fmt.Sprintf("Step %d of %d", visibleStepIndex(w)+1, countVisibleSteps(w)))

	if w.errMsg != "" {
		body += "\n\n" + errorBoxStyle.Render(w.errMsg)
	}

	help := [][2]string{{"Enter", "Next"}, {"Esc", "Cancel"}}
	return renderFrame(width, height, "GONIX", body, help)
}

func visibleStepIndex(w *wizardScreen) int {
	count := 0
	for i := 0; i < w.idx; i++ {
		f := w.fields[i]
		if f.ShowIf == nil || f.ShowIf(w.values) {
			count++
		}
	}
	return count
}

func countVisibleSteps(w *wizardScreen) int {
	count := 0
	for _, f := range w.fields {
		if f.ShowIf == nil || f.ShowIf(w.values) {
			count++
		}
	}
	return count
}
