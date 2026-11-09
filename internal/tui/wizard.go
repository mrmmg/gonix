package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// choiceSearchMin is how many options a choice question needs before it
// offers search.
const choiceSearchMin = 6

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
	// ChoiceDefault preselects the matching option of a fieldChoice; the
	// first option is preselected when it is empty or not found.
	ChoiceDefault string
	Validate      func(value string) error
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
	boolCursor int         // 0 = No, 1 = Yes
	menu       *simpleMenu // the options of a choice question
	errMsg     string

	onDone   func(values map[string]string) (screen, tea.Cmd)
	onCancel func() (screen, tea.Cmd)

	// path is the steps answered so far, so going back also works across
	// steps that were skipped.
	path []int
	// review, when set, shows what is about to change (in words, one line
	// each) after the last question and waits for a confirmation.
	review    func(values map[string]string) []string
	reviewing bool
}

// answersReview is the review of a wizard that has no summary of its own: the
// questions that were asked, each with its answer (secrets hidden).
func (w *wizardScreen) answersReview(values map[string]string) []string {
	var out []string
	for _, f := range w.fields {
		if f.ShowIf != nil && !f.ShowIf(values) {
			continue
		}
		v, ok := values[f.Key]
		if !ok {
			continue
		}
		switch {
		case f.Sensitive:
			v = "••••••••"
		case f.Kind == fieldBool && v == "yes":
			v = "Yes"
		case f.Kind == fieldBool:
			v = "No"
		case v == "":
			v = "(empty)"
		}
		out = append(out, strings.TrimSuffix(strings.TrimSpace(f.Label), ":")+" → "+v)
	}
	return out
}

// withReview makes the wizard end with a plain-language summary of the
// change, which must be confirmed before anything is applied.
func (w *wizardScreen) withReview(f func(values map[string]string) []string) *wizardScreen {
	w.review = f
	return w
}

func newWizard(title string, fields []wizardField, onDone func(map[string]string) (screen, tea.Cmd), onCancel func() (screen, tea.Cmd)) *wizardScreen {
	w := &wizardScreen{title: title, fields: fields, values: map[string]string{}, onDone: onDone, onCancel: onCancel}
	w.review = w.answersReview
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
		val := f.Default
		if prev, ok := w.values[f.Key]; ok {
			val = prev
		}
		ti.SetValue(val)
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
		if prev, ok := w.values[f.Key]; ok {
			if prev == "yes" {
				w.boolCursor = 1
			}
		} else if f.BoolDefault {
			w.boolCursor = 1
		}
	case fieldChoice:
		items := make([]menuItem, len(f.Options))
		for j, opt := range f.Options {
			items[j] = menuItem{title: opt}
		}
		// Any list of more than a few entries can be searched with Ctrl+F.
		if len(f.Options) >= choiceSearchMin {
			w.menu = newSearchMenu(items, "type to filter the choices")
		} else {
			w.menu = newSimpleMenu(items)
		}
		want := f.ChoiceDefault
		if prev, ok := w.values[f.Key]; ok {
			want = prev
		}
		for j, opt := range f.Options {
			if opt == want {
				w.menu.cursor = j
				break
			}
		}
	}
}

func (w *wizardScreen) currentField() (wizardField, bool) {
	if w.idx >= len(w.fields) {
		return wizardField{}, false
	}
	return w.fields[w.idx], true
}

func (w *wizardScreen) Init() tea.Cmd { return textinput.Blink }

// advance records the answer to the current step and moves on: to the next
// question, to the review, or to the end.
func (w *wizardScreen) advance(key, val string) (screen, tea.Cmd) {
	w.values[key] = val
	w.path = append(w.path, w.idx)
	w.enterStep(w.idx + 1)
	if _, more := w.currentField(); !more {
		return w.complete()
	}
	return w, textinput.Blink
}

// back returns to the question before the current one (or, from the review,
// to the last question), keeping every answer given so far.
func (w *wizardScreen) back() (screen, tea.Cmd) {
	if w.reviewing {
		w.reviewing = false
	}
	if len(w.path) == 0 {
		return w, nil
	}
	prev := w.path[len(w.path)-1]
	w.path = w.path[:len(w.path)-1]
	w.enterStep(prev)
	return w, textinput.Blink
}

// complete is reached after the last question.
func (w *wizardScreen) complete() (screen, tea.Cmd) {
	if w.review != nil {
		w.reviewing = true
		return w, nil
	}
	return w.finish()
}

func (w *wizardScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	km, isKey := msg.(tea.KeyMsg)
	if isKey && w.reviewing {
		switch km.String() {
		case "enter", "y":
			return w.finish()
		case "esc", "n":
			if w.onCancel != nil {
				return w.onCancel()
			}
			return w, navPop()
		case "shift+tab", "up", "left", "k", "h", "b":
			return w.back()
		}
		return w, nil
	}

	f, ok := w.currentField()
	if !ok {
		return w, nil
	}
	if !isKey {
		return w, nil
	}

	// While the search box of a choice list is open, it gets every key but
	// Enter (so Esc closes the search instead of cancelling the wizard).
	if f.Kind == fieldChoice && w.menu.Searching() && km.String() != "enter" {
		w.menu.HandleKey(km)
		return w, nil
	}

	switch km.String() {
	case "esc":
		if w.onCancel != nil {
			return w.onCancel()
		}
		return w, navPop()
	case "shift+tab", "ctrl+b":
		return w.back()
	}

	switch f.Kind {
	case fieldText:
		switch km.String() {
		case "enter":
			val := w.input.Value()
			if f.Validate != nil {
				if err := f.Validate(val); err != nil {
					w.errMsg = err.Error()
					return w, nil
				}
			}
			return w.advance(f.Key, val)
		case "up":
			// A single-line field has no use for up: it goes back.
			return w.back()
		default:
			var cmd tea.Cmd
			w.input, cmd = w.input.Update(msg)
			return w, cmd
		}

	case fieldBool:
		switch km.String() {
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
			return w.advance(f.Key, val)
		}

	case fieldChoice:
		if w.menu.HandleKey(km) {
			return w, nil
		}
		if km.String() == "enter" {
			if sel := w.menu.Selected(); sel >= 0 {
				return w.advance(f.Key, f.Options[sel])
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
	// Answers given before going back and changing a choice may belong to
	// questions that are no longer asked; they must not leak into the result.
	// Several questions may share a key (one per scope, say): a key stays
	// as long as one of them is asked.
	asked := map[string]bool{}
	for _, f := range w.fields {
		if f.ShowIf == nil || f.ShowIf(w.values) {
			asked[f.Key] = true
		}
	}
	for _, f := range w.fields {
		if !asked[f.Key] {
			delete(w.values, f.Key)
		}
	}
	next, cmd := w.onDone(w.values)
	return w, tea.Batch(cmd, navReplace(next))
}

func (w *wizardScreen) viewReview(width, height int) string {
	body := headerStyle.Render(w.title) + "\n"
	var lines []string
	for _, line := range w.review(w.values) {
		lines = append(lines, "• "+line)
	}
	body += callout(width-4, calloutInfo, "This is what will be applied", lines) + "\n"
	body += mutedStyle.Render("Nothing is changed until you apply. The configuration is tested first, and put back if the test fails.")
	help := [][2]string{{"Enter", "Apply"}, {"Shift+Tab/↑", "Back"}, {"Esc", "Cancel"}}
	return renderFrame(width, height, "GONIX", body, help)
}

func (w *wizardScreen) View(width, height int) string {
	if w.reviewing {
		return w.viewReview(width, height)
	}
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
		no, yes := normalItemStyle.Render("  No  "), normalItemStyle.Render("  Yes  ")
		if w.boolCursor == 0 {
			no = renderSelected("  No  ", 0)
		} else {
			yes = renderSelected("  Yes  ", 0)
		}
		body += no + "    " + yes
	case fieldChoice:
		body += w.menu.ViewFit(body, height-3)
	}

	body += "\n\n" + mutedStyle.Render(fmt.Sprintf("Step %d of %d", visibleStepIndex(w)+1, countVisibleSteps(w)))

	if w.errMsg != "" {
		body += "\n\n" + errorBoxStyle.Render(w.errMsg)
	}

	help := [][2]string{{"Enter", "Next"}}
	if len(w.path) > 0 {
		back := "Shift+Tab"
		if f.Kind == fieldText {
			back += "/↑"
		}
		help = append(help, [2]string{back, "Back"})
	}
	help = append(help, [2]string{"Esc", "Cancel"})
	if f.Kind == fieldChoice {
		help = searchHelp(w.menu, help)
	}
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
