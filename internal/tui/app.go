package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/accesslist"
	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/backup"
	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/system"
)

// Version is the GoNix release version, shown in the title bar. It
// is overridable at build time via -ldflags "-X ...Version=...".
var Version = "0.1.0"

// Deps bundles every backend dependency a screen might need. Screens must
// only interact with Nginx/the filesystem through these, never directly.
type Deps struct {
	Config      *config.Config
	Manager     *nginx.Manager
	HostService *hosts.Service
	Certs       string // certificate directory, screens call certificates.Scan themselves
	AccessLists *accesslist.Store
	SystemSvc   *system.Service
	Audit       *audit.Logger
	Backups     *backup.Backuper
}

// screen is implemented by every full-page view in the application. It
// mirrors tea.Model but returns the concrete screen type so the root model
// can manage a navigation stack.
type screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (screen, tea.Cmd)
	View(width, height int) string
}

// navPushMsg asks the root model to push a new screen onto the stack.
type navPushMsg struct{ screen screen }

// navPopMsg asks the root model to pop the current screen off the stack.
type navPopMsg struct{}

// navReplaceMsg asks the root model to replace only the current top-of-stack
// screen with a new one, leaving every screen below it untouched (used when
// a multi-step workflow, e.g. a wizard followed by its result screen,
// finishes and hands off to whatever should be shown next).
type navReplaceMsg struct{ screen screen }

func navPush(s screen) tea.Cmd    { return func() tea.Msg { return navPushMsg{s} } }
func navPop() tea.Cmd             { return func() tea.Msg { return navPopMsg{} } }
func navReplace(s screen) tea.Cmd { return func() tea.Msg { return navReplaceMsg{s} } }

// rootModel is the top-level Bubble Tea model. It owns the navigation stack
// and forwards all messages to the top-most screen.
type rootModel struct {
	deps   Deps
	stack  []screen
	width  int
	height int
}

// NewApp constructs the root Bubble Tea program model, starting at the main
// menu.
func NewApp(deps Deps) tea.Model {
	m := &rootModel{deps: deps}
	m.stack = []screen{newMainMenu(deps)}
	return m
}

func (m *rootModel) Init() tea.Cmd {
	return m.top().Init()
}

func (m *rootModel) top() screen {
	return m.stack[len(m.stack)-1]
}

func (m *rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		var cmd tea.Cmd
		newTop, cmd2 := m.top().Update(msg)
		m.stack[len(m.stack)-1] = newTop
		return m, tea.Batch(cmd, cmd2)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case navPushMsg:
		m.stack = append(m.stack, msg.screen)
		return m, tea.Batch(msg.screen.Init(), sizeCmd(m.width, m.height))

	case navPopMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, sizeCmd(m.width, m.height)

	case navReplaceMsg:
		if len(m.stack) > 0 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		m.stack = append(m.stack, msg.screen)
		return m, tea.Batch(msg.screen.Init(), sizeCmd(m.width, m.height))
	}

	newTop, cmd := m.top().Update(msg)
	m.stack[len(m.stack)-1] = newTop
	return m, cmd
}

func sizeCmd(w, h int) tea.Cmd {
	return func() tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} }
}

func (m *rootModel) View() string {
	if m.width == 0 {
		return "Loading GoNix..."
	}
	return m.top().View(m.width, m.height)
}

// backgroundCtx is used for the short-lived operations screens trigger
// (nginx -t, systemctl reload, ...). A future version may thread a proper
// cancellable context through the TUI; for now these actions are fast and
// synchronous from the user's perspective.
func backgroundCtx() context.Context { return context.Background() }
