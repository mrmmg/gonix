package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/system"
)

// statusScreen implements "Nginx Status": service control, process
// information and a lightweight monitoring overview.
type statusScreen struct {
	deps    Deps
	menu    *simpleMenu
	message string
	isErr   bool
}

const (
	stStart = iota
	stStop
	stRestart
	stReload
	stTest
)

func newStatusScreen(deps Deps) *statusScreen {
	s := &statusScreen{deps: deps}
	s.menu = newSimpleMenu([]menuItem{
		{title: "Start"},
		{title: "Stop"},
		{title: "Restart"},
		{title: "Reload"},
		{title: "Test Configuration"},
	})
	return s
}

func (s *statusScreen) Init() tea.Cmd { return nil }

func (s *statusScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "r":
			s.message, s.isErr = "", false
			return s, nil
		case "enter":
			return s.runAction(s.menu.Selected())
		}
	}
	return s, nil
}

func (s *statusScreen) runAction(choice int) (screen, tea.Cmd) {
	ctx := backgroundCtx()
	svc := s.deps.SystemSvc

	switch choice {
	case stTest:
		result, terr := nginx.NewValidator(svc.BinaryPath).Test(ctx)
		if terr != nil {
			s.message, s.isErr = terr.Error(), true
			return s, nil
		}
		if result.OK {
			s.message, s.isErr = "Configuration test passed.", false
		} else {
			s.message, s.isErr = result.Output, true
		}
		return s, nil
	case stStart:
		err := svc.Start(ctx)
		s.setResult("Nginx started.", err)
	case stStop:
		err := svc.Stop(ctx)
		s.setResult("Nginx stopped.", err)
	case stRestart:
		if !s.testFirst() {
			return s, nil
		}
		err := svc.Restart(ctx)
		s.setResult("Nginx restarted.", err)
	case stReload:
		if !s.testFirst() {
			return s, nil
		}
		err := svc.Reload(ctx)
		s.setResult("Nginx reloaded.", err)
	}
	return s, nil
}

// testFirst runs `nginx -t` before a restart/reload and blocks the action if
// it fails, matching the "never reload broken config" safety requirement.
func (s *statusScreen) testFirst() bool {
	result, err := nginx.NewValidator(s.deps.SystemSvc.BinaryPath).Test(backgroundCtx())
	if err != nil {
		s.message, s.isErr = err.Error(), true
		return false
	}
	if !result.OK {
		s.message, s.isErr = "Refusing to proceed: configuration test failed.\n"+result.Output, true
		return false
	}
	return true
}

func (s *statusScreen) setResult(okMsg string, err error) {
	if err != nil {
		s.message, s.isErr = err.Error(), true
		return
	}
	s.message, s.isErr = okMsg, false
}

func (s *statusScreen) View(width, height int) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Nginx Status") + "\n\n")

	status, _ := s.deps.SystemSvc.GetStatus(backgroundCtx())
	activeStr := dangerStyle.Render("STOPPED")
	if status.Active {
		activeStr = successStyle.Render("RUNNING")
	}
	enabledStr := mutedStyle.Render("disabled at boot")
	if status.Enabled {
		enabledStr = mutedStyle.Render("enabled at boot")
	}
	fmt.Fprintf(&b, "Service: %s  (%s)\n\n", activeStr, enabledStr)

	info, err := system.FindProcesses("/proc")
	if err == nil && info.MasterPID > 0 {
		fmt.Fprintf(&b, "Master PID: %d    Workers: %d    Total processes: %d\n", info.MasterPID, info.WorkerCount, info.TotalCount)
		if mem, merr := system.MemoryUsage("/proc", info.MasterPID); merr == nil {
			fmt.Fprintf(&b, "Master memory (RSS): %.1f MB\n", float64(mem)/1024/1024)
		}
		if up, uerr := system.Uptime("/proc", info.MasterPID); uerr == nil {
			fmt.Fprintf(&b, "Uptime: %s\n", up.Truncate(1e9))
		}
		ports, _ := system.ListeningPorts("/proc", append([]int{info.MasterPID}, info.WorkerPIDs...))
		if len(ports) > 0 {
			portsStr := make([]string, len(ports))
			for i, p := range ports {
				portsStr[i] = fmt.Sprintf("%d", p)
			}
			fmt.Fprintf(&b, "Listening ports: %s\n", strings.Join(portsStr, ", "))
		}
	} else {
		b.WriteString(mutedStyle.Render("No running Nginx processes detected.") + "\n")
	}

	b.WriteString("\n" + s.menu.View())

	if s.message != "" {
		b.WriteString("\n")
		if s.isErr {
			b.WriteString(errorBoxStyle.Render(s.message))
		} else {
			b.WriteString(successBoxStyle.Render(s.message))
		}
	}

	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Run"}, {"Esc", "Back"}}
	return renderFrame(width, height, "GONIX", b.String(), help)
}
