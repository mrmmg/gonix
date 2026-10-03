package errorpages

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
)

type fakeTester struct{ ok bool }

func (f fakeTester) Test(ctx context.Context) (nginx.Result, error) {
	return nginx.Result{OK: f.ok, Output: "fake nginx -t output"}, nil
}

func TestParseCodes(t *testing.T) {
	codes, err := ParseCodes("504, 502 502,503")
	if err != nil {
		t.Fatalf("ParseCodes: %v", err)
	}
	if want := []int{502, 503, 504}; !reflect.DeepEqual(codes, want) {
		t.Errorf("codes = %v, want %v", codes, want)
	}
	for _, bad := range []string{"", "abc", "200", "600"} {
		if _, err := ParseCodes(bad); err == nil {
			t.Errorf("ParseCodes(%q): expected error", bad)
		}
	}
}

func TestValidateDirectory(t *testing.T) {
	if err := ValidateDirectory("/var/www/html/error_pages"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	for _, bad := range []string{"", "relative/dir", "/a b", "/x;y", "/x}"} {
		if err := ValidateDirectory(bad); err == nil {
			t.Errorf("ValidateDirectory(%q): expected error", bad)
		}
	}
}

func TestRenderParseRoundTrip(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sn := Snippet{Name: "default", Directory: "/var/www/html/error_pages/", Codes: []int{502, 504}}
	out, err := store.Render(sn)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		"error_page 502 /__gonix_error_pages/502.html;",
		"error_page 504 /__gonix_error_pages/504.html;",
		"location ^~ /__gonix_error_pages/ {",
		"internal;",
		"auth_basic off;",
		"alias /var/www/html/error_pages/;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered snippet missing %q\n--- output ---\n%s", want, out)
		}
	}

	got := Parse("default", out)
	want := Snippet{Name: "default", Directory: "/var/www/html/error_pages", Codes: []int{502, 504}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %+v, want %+v", got, want)
	}
}

func newTestService(t *testing.T, ok bool) (*Service, *nginx.Manager) {
	t.Helper()
	snippets := t.TempDir()
	store, err := NewStore(snippets)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	renderer, err := nginx.NewRenderer(t.TempDir(), t.TempDir(), t.TempDir(), snippets)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	mgr := nginx.NewManager(t.TempDir(), t.TempDir(), renderer)
	return &Service{Store: store, Manager: mgr, Tester: fakeTester{ok: ok}}, mgr
}

func TestServiceCreateListAndMissingPages(t *testing.T) {
	svc, _ := newTestService(t, true)
	pages := t.TempDir()
	if err := os.WriteFile(filepath.Join(pages, "502.html"), []byte("<h1>502</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	sn := Snippet{Name: "default", Directory: pages, Codes: []int{502, 503}}
	if err := svc.Create(context.Background(), sn); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Create(context.Background(), sn); err == nil {
		t.Error("expected error creating a duplicate snippet")
	}

	list, err := svc.Store.List()
	if err != nil || len(list) != 1 || list[0].Name != "default" {
		t.Fatalf("List = %+v, %v", list, err)
	}
	missing := list[0].MissingPages()
	if len(missing) != 1 || !strings.HasSuffix(missing[0], "503.html") {
		t.Errorf("MissingPages = %v, want only 503.html", missing)
	}
}

func TestServiceRollsBackOnFailedTest(t *testing.T) {
	svc, _ := newTestService(t, true)
	orig := Snippet{Name: "default", Directory: "/srv/errors", Codes: []int{502}}
	if err := svc.Create(context.Background(), orig); err != nil {
		t.Fatalf("Create: %v", err)
	}

	svc.Tester = fakeTester{ok: false}
	if err := svc.Update(context.Background(), Snippet{Name: "default", Directory: "/srv/other", Codes: []int{404}}); err == nil {
		t.Fatal("expected Update to fail")
	}
	got, err := svc.Store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("snippet after failed update = %+v, want original %+v", got, orig)
	}

	if err := svc.Create(context.Background(), Snippet{Name: "fresh", Directory: "/srv/errors", Codes: []int{502}}); err == nil {
		t.Fatal("expected Create to fail")
	}
	if svc.Store.Exists("fresh") {
		t.Error("failed Create left its snippet file behind")
	}

	if err := svc.Delete(context.Background(), "default"); err == nil {
		t.Fatal("expected Delete to fail")
	}
	if !svc.Store.Exists("default") {
		t.Error("failed Delete did not restore the snippet")
	}
}

func TestServiceDeleteRefusesWhileInUse(t *testing.T) {
	svc, mgr := newTestService(t, true)
	if err := svc.Create(context.Background(), Snippet{Name: "default", Directory: "/srv/errors", Codes: []int{502}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	h := nginx.Host{
		ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80,
		ErrorPagesSnippet: "default",
		Locations: []nginx.Location{
			{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: "127.0.0.1", UpstreamPort: 8080, HTTPVersion: "1.1"}},
		},
	}
	if err := mgr.WriteHost(h); err != nil {
		t.Fatalf("WriteHost: %v", err)
	}

	err := svc.Delete(context.Background(), "default")
	if err == nil || !strings.Contains(err.Error(), "example.com") {
		t.Fatalf("Delete error = %v, want one naming example.com", err)
	}
	if !svc.Store.Exists("default") {
		t.Error("snippet was deleted while in use")
	}
}
