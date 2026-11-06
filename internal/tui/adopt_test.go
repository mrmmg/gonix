package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrmmg/gonix/internal/nginx"
)

type passTester struct{}

func (passTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func TestAdoptFromHostDetail(t *testing.T) {
	deps := testDeps(t)
	deps.HostService.Tester = passTester{}
	path := filepath.Join(deps.Config.Nginx.SitesAvailable, "site.conf")
	src := "server {\n    listen 80;\n    server_name hand.example.com;\n    root /srv;\n    index index.php;\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	list, _ := deps.Manager.ListHosts()
	if len(list) != 1 || list[0].Managed {
		t.Fatalf("hosts = %+v", list)
	}
	s := newHostDetailScreen(deps, list[0])
	if !s.selectAction(hdAdopt) {
		t.Fatal("Adopt Host is not offered for an unmanaged host")
	}
	next, _ := s.dispatch(hdAdopt)
	confirm, ok := next.(*confirmScreen)
	if !ok || !strings.Contains(confirm.message, "Only a marker comment is added") || !strings.Contains(confirm.message, "kept as written") {
		t.Fatalf("expected a confirmation, got %T %+v", next, next)
	}
	next, _ = confirm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if r, ok := next.(*resultScreen); !ok || !r.ok {
		t.Fatalf("adopt result = %+v", next)
	}
	data, _ := os.ReadFile(path)
	if !nginx.IsAdopted(string(data)) || !strings.HasSuffix(string(data), src) {
		t.Fatalf("file after adopt:\n%s", data)
	}
	if s.selectAction(hdAdopt) {
		t.Error("Adopt Host still offered after adopting")
	}
	if !strings.Contains(s.View(100, 40), "adopted: hand-written file site.conf") {
		t.Error("host detail does not say the host is adopted")
	}
}
