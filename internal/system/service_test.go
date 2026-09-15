package system

import (
	"context"
	"strings"
	"testing"
)

type fakeRunner struct {
	responses map[string]string
	errs      map[string]error
	calls     []string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	return f.responses[key], f.errs[key]
}

func TestGetStatus(t *testing.T) {
	runner := &fakeRunner{responses: map[string]string{
		"systemctl is-active nginx":  "active\n",
		"systemctl is-enabled nginx": "enabled\n",
	}}
	svc := &Service{Runner: runner, UnitName: "nginx"}

	st, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !st.Active {
		t.Error("expected Active to be true")
	}
	if !st.Enabled {
		t.Error("expected Enabled to be true")
	}
}

func TestGetStatusInactive(t *testing.T) {
	runner := &fakeRunner{responses: map[string]string{
		"systemctl is-active nginx":  "inactive\n",
		"systemctl is-enabled nginx": "disabled\n",
	}}
	svc := &Service{Runner: runner, UnitName: "nginx"}

	st, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.Active {
		t.Error("expected Active to be false")
	}
	if st.Enabled {
		t.Error("expected Enabled to be false")
	}
}

func TestReloadUsesSystemctl(t *testing.T) {
	runner := &fakeRunner{responses: map[string]string{}}
	svc := &Service{Runner: runner, UnitName: "nginx"}
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	found := false
	for _, c := range runner.calls {
		if c == "systemctl reload nginx" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected systemctl reload nginx to be called, got %v", runner.calls)
	}
}
