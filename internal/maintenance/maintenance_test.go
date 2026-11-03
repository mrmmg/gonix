package maintenance

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/backup"
	"github.com/mrmmg/gonix/internal/errorpages"
	"github.com/mrmmg/gonix/internal/hosts"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/nginxtest"
	"github.com/mrmmg/gonix/internal/snippet"
)

func TestRenderParseBypass(t *testing.T) {
	bypass := []string{"10.0.0.0/8", "203.0.113.5", "2001:db8::/32"}
	out, err := Render("my-site.example.com", bypass)
	if err != nil {
		t.Fatal(err)
	}
	if got := ParseBypass(out); !reflect.DeepEqual(got, bypass) {
		t.Errorf("ParseBypass = %v, want %v\n%s", got, bypass, out)
	}
	if !strings.Contains(out, "$gonix_maint_my__site_example_com") {
		t.Errorf("unexpected variable name:\n%s", out)
	}
	if _, err := Render("example.com", []string{"10.0.0.1; deny all"}); err == nil {
		t.Error("expected an invalid bypass address to be rejected")
	}
	if got := ParseBypass(mustRender(t, "example.com", nil)); len(got) != 0 {
		t.Errorf("ParseBypass without bypass = %v", got)
	}
}

func mustRender(t *testing.T, name string, bypass []string) string {
	t.Helper()
	out, err := Render(name, bypass)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func TestServiceEnableDisable(t *testing.T) {
	r, err := nginx.NewRenderer(t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mgr := nginx.NewManager(t.TempDir(), t.TempDir(), r)
	auditLog, _ := audit.NewLogger(filepath.Join(t.TempDir(), "audit.log"))
	hostSvc := &hosts.Service{Manager: mgr, Tester: okTester{}, Backups: backup.New(t.TempDir(), 5), Audit: auditLog}
	svc := &Service{HTTPDir: t.TempDir(), Hosts: hostSvc, Writer: &snippet.Writer{Tester: okTester{}}}
	ctx := context.Background()

	if err := mgr.WriteHost(nginx.Host{ServerName: "example.com", Mode: nginx.ModeStatic, Root: "/srv", Listen: 80}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Enable(ctx, "example.com", []string{"10.0.0.1"}); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	h, _ := mgr.GetHost("example.com")
	if !h.Maintenance || !reflect.DeepEqual(svc.Bypass("example.com"), []string{"10.0.0.1"}) {
		t.Errorf("after Enable: maintenance=%v bypass=%v", h.Maintenance, svc.Bypass("example.com"))
	}
	if err := svc.Disable(ctx, "example.com"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	h, _ = mgr.GetHost("example.com")
	if h.Maintenance {
		t.Error("host still in maintenance after Disable")
	}
	if _, err := os.Stat(svc.Path("example.com")); !os.IsNotExist(err) {
		t.Error("http-level file should be removed on Disable")
	}

	if err := mgr.WriteHost(nginx.Host{ServerName: "old.example.com", Mode: nginx.ModeRedirect, Listen: 80,
		Redirect: nginx.RedirectConfig{Target: "https://example.com", Code: 301}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Enable(ctx, "old.example.com", nil); err == nil {
		t.Error("expected maintenance to be refused for a redirect host")
	}
}

// TestMaintenanceBehavior runs a real Nginx: clients get 503 with the
// host's custom error page, while bypass addresses still reach the site.
func TestMaintenanceBehavior(t *testing.T) {
	for _, tc := range []struct {
		name       string
		bypass     []string
		wantStatus int
		wantBody   string
	}{
		{"closed", nil, http.StatusServiceUnavailable, "back soon"},
		{"bypass", []string{"127.0.0.0/8"}, http.StatusOK, "live site"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := nginxtest.New(t)
			root := filepath.Join(p.Dir, "www")
			pages := filepath.Join(p.Dir, "pages")
			snippets := filepath.Join(p.Dir, "snippets")
			for dir, file := range map[string]string{root: "index.html", pages: "503.html"} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				body := "live site"
				if file == "503.html" {
					body = "back soon"
				}
				if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			store, err := errorpages.NewStore(snippets)
			if err != nil {
				t.Fatal(err)
			}
			sn, err := store.Render(errorpages.Snippet{Name: "default", Directory: pages, Codes: []int{503}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.Path("default"), []byte(sn), 0o644); err != nil {
				t.Fatal(err)
			}

			r, err := nginx.NewRenderer(p.Dir, p.Dir, p.Dir, snippets)
			if err != nil {
				t.Fatal(err)
			}
			h := nginx.Host{ServerName: "example.com", Mode: nginx.ModeStatic, Root: root, Listen: 80,
				ErrorPagesSnippet: "default", Maintenance: true}
			out, err := r.Render(h)
			if err != nil {
				t.Fatal(err)
			}
			p.AddSite(t, "example.com", out)
			p.AddHTTP(t, nginx.MaintenanceFileName("example.com"), mustRender(t, "example.com", tc.bypass))
			p.Start(t)

			req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(p.Port(t, 80))+"/", nil)
			req.Host = "example.com"
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != tc.wantStatus || !strings.Contains(string(body), tc.wantBody) {
				t.Errorf("GET / = %d %q, want %d containing %q", resp.StatusCode, body, tc.wantStatus, tc.wantBody)
			}
		})
	}
}
