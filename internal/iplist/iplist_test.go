package iplist

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/nginxtest"
	"github.com/mrmmg/gonix/internal/snippet"
)

func TestRenderParseRoundTrip(t *testing.T) {
	for _, l := range []List{
		{Name: "office", Mode: Allowlist, Entries: []string{"192.168.1.0/24", "10.0.0.5", "2001:db8::/32"}},
		{Name: "bad-bots", Mode: Blocklist, Entries: []string{"203.0.113.7"}},
	} {
		out, err := Render(l)
		if err != nil {
			t.Fatalf("Render(%s): %v", l.Name, err)
		}
		if got := Parse(l.Name, out); !reflect.DeepEqual(got, l) {
			t.Errorf("round trip = %+v, want %+v\n%s", got, l, out)
		}
	}

	out, _ := Render(List{Name: "office", Mode: Allowlist, Entries: []string{"10.0.0.5"}})
	if !strings.Contains(out, "allow 10.0.0.5;\ndeny all;") {
		t.Errorf("allowlist must allow entries then deny everyone else:\n%s", out)
	}
	out, _ = Render(List{Name: "bl", Mode: Blocklist, Entries: []string{"10.0.0.5"}})
	if !strings.Contains(out, "deny 10.0.0.5;\nallow all;") {
		t.Errorf("blocklist must deny entries then allow everyone else:\n%s", out)
	}
}

func TestValidation(t *testing.T) {
	if _, err := ParseEntries("10.0.0.1, 192.168.0.0/16 10.0.0.1 ::1"); err != nil {
		t.Errorf("ParseEntries: %v", err)
	}
	for _, bad := range []string{"", "10.0.0.256", "10.0.0.0/33", "example.com", "all", "10.0.0.1;deny"} {
		if _, err := ParseEntries(bad); err == nil {
			t.Errorf("ParseEntries(%q) = nil error, want error", bad)
		}
	}
	for _, l := range []List{
		{Name: "../evil", Mode: Allowlist, Entries: []string{"10.0.0.1"}},
		{Name: "x", Mode: "both", Entries: []string{"10.0.0.1"}},
		{Name: "x", Mode: Allowlist},
	} {
		if err := l.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want error", l)
		}
	}
}

func TestContains(t *testing.T) {
	l := List{Entries: []string{"192.168.1.0/24", "10.0.0.5"}}
	for ip, want := range map[string]bool{"192.168.1.77": true, "10.0.0.5": true, "10.0.0.6": false, "junk": false} {
		if got := l.Contains(ip); got != want {
			t.Errorf("Contains(%s) = %v, want %v", ip, got, want)
		}
	}
}

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func newService(t *testing.T) (*Service, *nginx.Manager) {
	t.Helper()
	snippets := t.TempDir()
	store, err := NewStore(snippets)
	if err != nil {
		t.Fatal(err)
	}
	r, err := nginx.NewRenderer(t.TempDir(), t.TempDir(), t.TempDir(), snippets)
	if err != nil {
		t.Fatal(err)
	}
	mgr := nginx.NewManager(t.TempDir(), t.TempDir(), r)
	return &Service{Store: store, Manager: mgr, Writer: &snippet.Writer{Tester: okTester{}}}, mgr
}

func TestServiceLifecycle(t *testing.T) {
	svc, mgr := newService(t)
	ctx := context.Background()
	l := List{Name: "office", Mode: Allowlist, Entries: []string{"10.0.0.0/8"}}

	if err := svc.Create(ctx, l); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Create(ctx, l); err == nil {
		t.Error("expected duplicate Create to fail")
	}
	if err := svc.AddEntry(ctx, "office", "203.0.113.9"); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	if err := svc.AddEntry(ctx, "office", "203.0.113.9"); err != nil {
		t.Fatalf("AddEntry (duplicate): %v", err)
	}
	got, err := svc.Store.Get("office")
	if err != nil || len(got.Entries) != 2 {
		t.Fatalf("Get = %+v, %v; want 2 entries", got, err)
	}

	h := nginx.Host{
		ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80,
		Locations: []nginx.Location{{Path: "/admin/", IPListName: "office", Proxy: &nginx.ProxyConfig{
			UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}}},
	}
	if err := mgr.WriteHost(h); err != nil {
		t.Fatal(err)
	}
	users, _ := svc.UsedBy("office")
	if !reflect.DeepEqual(users, []string{"example.com /admin/"}) {
		t.Errorf("UsedBy = %v", users)
	}
	if err := svc.Delete(ctx, "office"); err == nil {
		t.Error("expected Delete to refuse a list that is still in use")
	}
	if err := os.Remove(filepath.Join(mgr.SitesAvailable, "example.com")); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "office"); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if svc.Store.Exists("office") {
		t.Error("list file should be gone")
	}
}

func TestHostWithIPListsPassesRealNginx(t *testing.T) {
	p := nginxtest.New(t)
	snippets := filepath.Join(p.Dir, "snippets")
	store, err := NewStore(snippets)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []List{
		{Name: "office", Mode: Allowlist, Entries: []string{"192.168.1.0/24", "2001:db8::/32"}},
		{Name: "blocked", Mode: Blocklist, Entries: []string{"203.0.113.7"}},
	} {
		out, err := Render(l)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.Path(l.Name), []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r, err := nginx.NewRenderer(p.Dir, p.Dir, p.Dir, snippets)
	if err != nil {
		t.Fatal(err)
	}
	h := nginx.Host{
		ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80,
		IPListName: "blocked", SatisfyAny: true,
		Locations: []nginx.Location{{Path: "/admin/", IPListName: "office", Proxy: &nginx.ProxyConfig{
			UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}}},
	}
	out, err := r.Render(h)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := nginx.ParseManagedHost(out)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.IPListName != "blocked" || !parsed.SatisfyAny || parsed.Locations[0].IPListName != "office" {
		t.Errorf("round trip lost IP list settings: %+v / %+v", parsed, parsed.Locations[0])
	}
	p.AddSite(t, "example.com", out)
	p.Test(t)
}

// TestIPListBehavior checks with a real Nginx that a blocklist refuses the
// listed client and an allowlist admits it.
func TestIPListBehavior(t *testing.T) {
	for _, tc := range []struct {
		mode Mode
		want int
	}{
		{Blocklist, http.StatusForbidden},
		{Allowlist, http.StatusOK},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			p := nginxtest.New(t)
			snippets := filepath.Join(p.Dir, "snippets")
			store, err := NewStore(snippets)
			if err != nil {
				t.Fatal(err)
			}
			out, err := Render(List{Name: "local", Mode: tc.mode, Entries: []string{"127.0.0.1"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.Path("local"), []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}
			r, err := nginx.NewRenderer(p.Dir, p.Dir, p.Dir, snippets)
			if err != nil {
				t.Fatal(err)
			}
			// A static file, not `return`: return answers before the access
			// phase, where allow/deny are evaluated.
			if err := os.WriteFile(filepath.Join(p.Dir, "index.html"), []byte("ok"), 0o644); err != nil {
				t.Fatal(err)
			}
			site, err := r.Render(nginx.Host{ServerName: "example.com", Mode: nginx.ModeStatic, Root: p.Dir, Listen: 80,
				IPListName: "local"})
			if err != nil {
				t.Fatal(err)
			}
			p.AddSite(t, "example.com", site)
			p.Start(t)

			req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(p.Port(t, 80))+"/", nil)
			req.Host = "example.com"
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestRemoveEntry(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	svc.Create(ctx, List{Name: "x", Mode: Blocklist, Entries: []string{"10.0.0.1", "10.0.0.2"}})
	if err := svc.RemoveEntry(ctx, "x", "10.0.0.9"); err == nil {
		t.Error("expected an error for an absent entry")
	}
	if err := svc.RemoveEntry(ctx, "x", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveEntry(ctx, "x", "10.0.0.2"); err == nil {
		t.Error("expected the last entry to be kept")
	}
	if l, _ := svc.Store.Get("x"); len(l.Entries) != 1 || l.Entries[0] != "10.0.0.2" {
		t.Errorf("entries = %v", l.Entries)
	}
}
