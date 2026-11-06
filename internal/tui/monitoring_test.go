package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/config"
)

func TestMonitoringScreenRenders(t *testing.T) {
	s := newMonitoringScreen(testDeps(t))
	out := s.View(100, 40)
	for _, want := range []string{"Monitoring", "/metrics", "Save Grafana Dashboard", "Change Settings"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen lacks %q:\n%s", want, out)
		}
	}
}

func TestSaveGrafanaDashboardFromTheTUI(t *testing.T) {
	deps := testDeps(t)
	w := newMonitoringScreen(deps).dashboardWizard().(*wizardScreen)
	dir := t.TempDir()
	next, _ := w.onDone(map[string]string{"path": dir})
	res, ok := next.(*resultScreen)
	if !ok || !res.ok {
		t.Fatalf("result = %#v", next)
	}
	data, err := os.ReadFile(filepath.Join(dir, "gonix-dashboard.json"))
	if err != nil || !strings.Contains(string(data), `"uid": "gonix-overview"`) {
		t.Errorf("dashboard not saved: %v", err)
	}
	if !strings.Contains(res.message, dir) {
		t.Errorf("message does not say where: %s", res.message)
	}
}

func TestSettingsWizardAppliesAndSaves(t *testing.T) {
	deps := testDeps(t)
	w := newMonitoringScreen(deps).settingsWizard().(*wizardScreen)
	listen := "127.0.0.1:" + "39113"
	next, _ := w.onDone(map[string]string{
		"listen": listen, "hosts": "*.example.com", "exclude": "", "collectors": strings.Join(config.Collectors, " "),
		"ttl": "45s", "stub": "no",
	})
	if res, ok := next.(*resultScreen); !ok || !res.ok {
		t.Fatalf("result = %#v", next)
	}
	loaded, err := config.Load(deps.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	m := loaded.Monitoring
	if m.Listen != listen || len(m.Hosts) != 1 || m.CacheTTL.Seconds() != 45 || len(m.Collectors) != 0 {
		t.Errorf("saved %+v (all collectors listed should be stored as 'all')", m)
	}
}
