package geoblock

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

func TestParseCountries(t *testing.T) {
	got, err := ParseCountries("de, us fr DE")
	if err != nil || !reflect.DeepEqual(got, []string{"DE", "FR", "US"}) {
		t.Errorf("ParseCountries = %v, %v", got, err)
	}
	for _, bad := range []string{"", "DEU", "d", "D1", "de;"} {
		if _, err := ParseCountries(bad); err == nil {
			t.Errorf("ParseCountries(%q) should fail", bad)
		}
	}
}

func TestRenderParse(t *testing.T) {
	for _, l := range []List{
		{Name: "eu_only", Mode: Allowlist, Countries: []string{"DE", "FR"}},
		{Name: "blocked", Mode: Blocklist, Countries: []string{"XX"}},
	} {
		m, err := RenderMap(l)
		if err != nil {
			t.Fatal(err)
		}
		if got := Parse(l.Name, m); !reflect.DeepEqual(got, l) {
			t.Errorf("round trip = %+v, want %+v\n%s", got, l, m)
		}
	}
	if err := (List{Name: "a-b", Mode: Allowlist, Countries: []string{"DE"}}).Validate(); err == nil {
		t.Error("hyphenated names must be rejected (they become variable names)")
	}
	if _, err := RenderSetup("/x; evil"); err == nil {
		t.Error("expected an unsafe database path to be rejected")
	}
	if out, _ := RenderSetup("/usr/share/GeoIP/GeoLite2-Country.mmdb"); !strings.Contains(out, "$gonix_country_code country iso_code;") {
		t.Errorf("setup = %s", out)
	}
}

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func TestServiceLifecycle(t *testing.T) {
	db := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	r, _ := nginx.NewRenderer(t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir())
	mgr := nginx.NewManager(t.TempDir(), t.TempDir(), r)
	svc := &Service{HTTPDir: t.TempDir(), SnippetsDir: t.TempDir(), Database: db, Manager: mgr, Writer: &snippet.Writer{Tester: okTester{}}}
	ctx := context.Background()
	l := List{Name: "eu", Mode: Allowlist, Countries: []string{"DE"}}

	if err := svc.Save(ctx, l, true); err == nil {
		t.Error("creating a list before setup should fail")
	}
	if err := svc.SetUp(ctx); err == nil {
		t.Error("setup without a database should fail")
	}
	os.WriteFile(db, []byte("fake"), 0o644)
	if err := svc.SetUp(ctx); err != nil {
		t.Fatal(err)
	}
	if ready, path := svc.Status(); !ready || path != db {
		t.Errorf("Status = %v %q", ready, path)
	}
	if err := svc.Save(ctx, l, true); err != nil {
		t.Fatal(err)
	}
	if lists, _ := svc.List(); len(lists) != 1 || !reflect.DeepEqual(lists[0], l) {
		t.Errorf("List = %+v", lists)
	}
	mgr.WriteHost(nginx.Host{ServerName: "example.com", Mode: nginx.ModeStatic, Root: "/srv", Listen: 80, CountryList: "eu"})
	if h, _ := mgr.GetHost("example.com"); h.CountryList != "eu" {
		t.Errorf("host CountryList = %q", h.CountryList)
	}
	if err := svc.Delete(ctx, "eu"); err == nil {
		t.Error("expected Delete to refuse a list in use")
	}
}

// TestCountryBlockingBehavior runs a real Nginx. The geoip2 module is not
// needed: $gonix_country_code comes from a test header instead of the
// database, which exercises everything GoNix generates except the geoip2
// block itself.
func TestCountryBlockingBehavior(t *testing.T) {
	p := nginxtest.New(t)
	snippets := filepath.Join(p.Dir, "snippets")
	os.MkdirAll(snippets, 0o755)
	p.AddHTTP(t, "0-test-country.conf", "map $http_x_test_country $"+CountryVar+" {\n    default $http_x_test_country;\n}\n")
	l := List{Name: "eu_only", Mode: Allowlist, Countries: []string{"DE", "FR"}}
	m, _ := RenderMap(l)
	c, _ := RenderCheck(l)
	p.AddHTTP(t, nginx.CountryListFileName(l.Name), m)
	os.WriteFile(filepath.Join(snippets, nginx.CountryListFileName(l.Name)), []byte(c), 0o644)
	os.WriteFile(filepath.Join(p.Dir, "index.html"), []byte("ok"), 0o644)

	r, _ := nginx.NewRenderer(p.Dir, p.Dir, p.Dir, snippets)
	site, err := r.Render(nginx.Host{ServerName: "example.com", Mode: nginx.ModeStatic, Root: p.Dir, Listen: 80, CountryList: l.Name})
	if err != nil {
		t.Fatal(err)
	}
	p.AddSite(t, "example.com", site)
	p.Start(t)

	for country, want := range map[string]int{"DE": 200, "FR": 200, "CN": 403, "": 403} {
		req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(p.Port(t, 80))+"/", nil)
		req.Host = "example.com"
		if country != "" {
			req.Header.Set("X-Test-Country", country)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("country %q: status %d, want %d", country, resp.StatusCode, want)
		}
	}
}
