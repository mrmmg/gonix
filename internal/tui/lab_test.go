package tui

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/lab"
)

// fakeLab is a lab in temporary directories with systemctl and nginx -t
// faked.
func fakeLab(t *testing.T) func() *lab.Lab {
	root := t.TempDir()
	active := false
	return func() *lab.Lab {
		l := lab.New()
		l.InstanceFile = filepath.Join(root, "instances", "lab.yaml")
		l.UnitDir = filepath.Join(root, "systemd")
		l.RequireRoot = false
		l.PortFree = func(int) bool { return true }
		l.WorkerGroup = func(context.Context) string { return "nobody" }
		l.TestConfig = func(context.Context, string) (string, error) { return "ok", nil }
		l.Systemctl = func(_ context.Context, args ...string) (string, error) {
			switch args[0] {
			case "start":
				active = true
			case "stop":
				active = false
			case "is-active":
				if active {
					return "active", nil
				}
				return "inactive", nil
			}
			return "", nil
		}
		return l
	}
}

func TestLabMenuNeedsEnvVar(t *testing.T) {
	deps := testDeps(t)
	t.Setenv(lab.EnvVar, "")
	if slices.Contains(newMainMenu(deps).actions, miLab) {
		t.Error("Lab offered without " + lab.EnvVar)
	}
	t.Setenv(lab.EnvVar, "1")
	if !slices.Contains(newMainMenu(deps).actions, miLab) {
		t.Error("Lab not offered with " + lab.EnvVar + "=1")
	}
}

func TestLabSetupEnterExit(t *testing.T) {
	t.Setenv(lab.EnvVar, "1")
	deps := testDeps(t)
	oldNew, oldBuild := newLab, buildLabDeps
	t.Cleanup(func() { newLab, buildLabDeps, inLab, instanceLabel = oldNew, oldBuild, false, "" })
	newLab = fakeLab(t)
	labDeps := testDeps(t)
	labDeps.Config.Instance = lab.Instance
	buildLabDeps = func(*lab.Lab) (Deps, error) { return labDeps, nil }

	s := newLabScreen(deps)
	if s.state != nil || s.actions[0] != laSetup {
		t.Fatalf("fresh lab screen: %+v", s.actions)
	}
	w, _ := s.dispatch(laSetup)
	wiz := w.(*wizardScreen)
	dir := filepath.Join(t.TempDir(), "lab")
	next, _ := wiz.onDone(map[string]string{"dir": dir, "http": "18080", "https": "18443", "samples": "yes"})
	if r := next.(*resultScreen); !r.ok || !strings.Contains(r.message, dir) {
		t.Fatalf("setup result: %+v", r)
	}

	s = newLabScreen(deps)
	if s.state == nil || !s.active || len(s.state.Hosts) != len(lab.Scenarios) || !slices.Contains(s.actions, laEnter) {
		t.Fatalf("lab screen after setup: state %+v active %v actions %v", s.state, s.active, s.actions)
	}
	if v := s.View(120, 40); !strings.Contains(v, "running") {
		t.Errorf("view:\n%s", v)
	}
	guide, _ := s.dispatch(laGuide)
	if !strings.Contains(strings.Join(guide.(*viewerScreen).lines, "\n"), "--instance lab") {
		t.Error("guide lacks the CLI usage")
	}

	// A test host of a chosen kind, with the suggested domain.
	add, _ := s.dispatch(laAddHost)
	next, _ = add.(*wizardScreen).onDone(map[string]string{"kind": lab.Scenarios[1].Title, "domain": "extra.lab.test"})
	if r := next.(*resultScreen); !r.ok {
		t.Fatalf("add host: %s", r.message)
	}

	// Enter: the TUI works on the lab; Exit Lab comes back.
	m := NewApp(deps).(*rootModel)
	_, cmd := s.dispatch(laEnter)
	model, _ := m.Update(cmd())
	m = model.(*rootModel)
	menu, ok := m.top().(*mainMenu)
	if !ok || !inLab || slices.Contains(menu.actions, miLab) || !strings.Contains(menu.View(120, 40), "LAB:") || !strings.Contains(menu.View(120, 40), "· lab") {
		t.Fatalf("in lab: top %T, inLab %v", m.top(), inLab)
	}
	_, cmd = menu.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model, _ = m.Update(cmd())
	m = model.(*rootModel)
	if _, ok := m.top().(*labScreen); !ok || inLab || instanceLabel != "" || len(m.stack) != 2 {
		t.Fatalf("after exit: top %T, inLab %v, label %q, stack %d", m.top(), inLab, instanceLabel, len(m.stack))
	}

	// Remove.
	s = newLabScreen(deps)
	c, _ := s.dispatch(laRemove)
	next, _ = c.(*confirmScreen).onYes()
	if r := next.(*resultScreen); !r.ok {
		t.Fatalf("remove: %s", r.message)
	}
	if s = newLabScreen(deps); s.state != nil {
		t.Error("lab still there after removal")
	}
}
