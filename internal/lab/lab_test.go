package lab

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/nginx"
)

// freePort returns a TCP port nothing listens on right now.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// testLab is a Lab writing into temporary directories, with a recorded
// systemctl and, when nginx is installed, the real nginx -t.
func testLab(t *testing.T) (*Lab, *[]string) {
	t.Helper()
	root := t.TempDir()
	var calls []string
	l := New()
	l.InstanceFile = filepath.Join(root, "instances", "lab.yaml")
	l.UnitDir = filepath.Join(root, "systemd")
	l.RequireRoot = false
	l.PortFree = func(int) bool { return true }
	l.WorkerGroup = func(context.Context) string { return "nobody" }
	active := false
	l.Systemctl = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		switch args[0] {
		case "start":
			active = true
		case "stop":
			active = false
		case "is-active":
			if active {
				return "active", nil
			}
			return "inactive", exec.ErrNotFound
		}
		return "", nil
	}
	if _, err := exec.LookPath("nginx"); err != nil {
		l.TestConfig = func(context.Context, string) (string, error) { return "skipped", nil }
	}
	return l, &calls
}

func TestSetupAddRemove(t *testing.T) {
	l, calls := testLab(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "lab")
	o := Options{Dir: dir, HTTPPort: freePort(t), HTTPSPort: freePort(t), Samples: true}
	st, err := l.Setup(ctx, o)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if len(st.Hosts) != len(Scenarios) {
		t.Fatalf("hosts = %+v", st.Hosts)
	}
	for _, f := range []string{"nginx.conf", "gonix.yaml", "lab.json", "conf.d/lab-backends.conf", "sites-enabled/default"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	unit, _ := os.ReadFile(filepath.Join(l.UnitDir, Unit+".service"))
	if !strings.Contains(string(unit), "-c "+dir+"/nginx.conf") {
		t.Errorf("unit:\n%s", unit)
	}
	if !slices.Contains(*calls, "daemon-reload") || !slices.Contains(*calls, "start "+Unit) || !slices.Contains(*calls, "reload "+Unit) {
		t.Errorf("systemctl calls = %q", *calls)
	}

	// The lab's state is found again through the instance file.
	loaded, err := l.Load()
	if err != nil || loaded == nil || loaded.Dir != dir || len(loaded.Hosts) != len(Scenarios) {
		t.Fatalf("Load = %+v, %v", loaded, err)
	}

	// Every scenario adopts (or is refused) as its description says.
	r, _ := nginx.NewRenderer(dir+"/logs", dir+"/logs", dir+"/accesslists", dir+"/snippets")
	for _, h := range st.Hosts {
		sc, _ := ScenarioByID(h.Scenario)
		content, _ := os.ReadFile(filepath.Join(dir, "sites-available", h.File))
		rep, err := r.InspectAdoption(h.File, string(content))
		refused := strings.HasPrefix(sc.Adopt, "refused")
		switch {
		case refused && sc.ID == "multi" && err == nil:
			t.Errorf("%s should not be adoptable", sc.ID)
		case !refused && err != nil:
			t.Errorf("%s: %v", sc.ID, err)
		case sc.ID == "certbot-proxy":
			if l, ok := certificates.CertbotLineage(rep.Host.SSL.CertFile); !ok || l.Authenticator != "nginx" || !rep.HTTPBlock {
				t.Errorf("certbot host: lineage %+v %v, report %+v", l, ok, rep)
			}
			if !strings.Contains(string(content), "return 301 https://$host:"+strconv.Itoa(o.HTTPSPort)+"$request_uri;") {
				t.Errorf("certbot redirect without the lab port:\n%s", content)
			}
		case sc.ID == "static" && rep.Host.Mode != nginx.ModeStatic:
			t.Errorf("static host mode = %s", rep.Host.Mode)
		case sc.ID == "default" && rep.Host.ServerName != "_":
			t.Errorf("default host = %+v", rep.Host)
		}
	}

	// A second host of the same name is refused; a bad one is rolled back.
	if _, err := l.AddHost(ctx, st, "proxy", "api.lab.test"); err == nil {
		t.Error("duplicate host accepted")
	}
	if _, err := l.AddHost(ctx, st, "proxy", "not a domain"); err == nil {
		t.Error("invalid domain accepted")
	}

	guide := l.Guide(ctx, st)
	for _, want := range []string{dir, "--instance lab", "app.lab.test", "refused: two server blocks"} {
		if !strings.Contains(guide, want) {
			t.Errorf("guide lacks %q", want)
		}
	}

	// Setting up again in the same directory resets it; elsewhere is refused.
	if _, err := l.Setup(ctx, Options{Dir: filepath.Join(t.TempDir(), "other"), HTTPPort: o.HTTPPort, HTTPSPort: o.HTTPSPort}); err == nil {
		t.Error("a second lab elsewhere was accepted")
	}
	st, err = l.Setup(ctx, Options{Dir: dir, HTTPPort: o.HTTPPort, HTTPSPort: o.HTTPSPort})
	if err != nil || len(st.Hosts) != 0 {
		t.Fatalf("reset: %+v, %v", st, err)
	}

	if err := l.Remove(ctx); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dir, l.InstanceFile, filepath.Join(l.UnitDir, Unit+".service")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists", p)
		}
	}
	if st, err := l.Load(); st != nil || err != nil {
		t.Errorf("after remove: %+v, %v", st, err)
	}
}

func TestSetupRefusals(t *testing.T) {
	l, _ := testLab(t)
	ctx := context.Background()
	notEmpty := t.TempDir()
	os.WriteFile(filepath.Join(notEmpty, "keep.txt"), []byte("x"), 0o644)
	for name, o := range map[string]Options{
		"relative":  {Dir: "lab", HTTPPort: 8080, HTTPSPort: 8443},
		"too high":  {Dir: "/srv", HTTPPort: 8080, HTTPSPort: 8443},
		"not empty": {Dir: notEmpty, HTTPPort: 8080, HTTPSPort: 8443},
		"same port": {Dir: filepath.Join(t.TempDir(), "x"), HTTPPort: 8080, HTTPSPort: 8080},
	} {
		if _, err := l.Setup(ctx, o); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	l.PortFree = func(int) bool { return false }
	if _, err := l.Setup(ctx, Options{Dir: filepath.Join(t.TempDir(), "x"), HTTPPort: 8080, HTTPSPort: 8443}); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Errorf("busy port: %v", err)
	}
	// An instance named "lab" the lab did not create is left alone.
	os.MkdirAll(filepath.Dir(l.InstanceFile), 0o755)
	os.WriteFile(l.InstanceFile, []byte("nginx:\n  config_dir: /etc/nginx\n"), 0o644)
	if _, err := l.Load(); err == nil {
		t.Error("foreign instance accepted")
	}
	if err := l.Remove(ctx); err == nil {
		t.Error("removing a foreign instance should fail")
	}
	if _, err := os.Stat(l.InstanceFile); err != nil {
		t.Error("a foreign instance file was deleted")
	}
	os.MkdirAll(l.UnitDir, 0o755)
	os.WriteFile(filepath.Join(l.UnitDir, Unit+".service"), []byte("x"), 0o644)
	l.Remove(ctx)
	if _, err := os.Stat(filepath.Join(l.UnitDir, Unit+".service")); err != nil {
		t.Error("the unit was deleted although the lab could not vouch for the instance")
	}
}

func TestEnabled(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "false": false, "1": true, "yes": true} {
		t.Setenv(EnvVar, v)
		if Enabled() != want {
			t.Errorf("%s=%q: Enabled() = %v", EnvVar, v, !want)
		}
	}
}
