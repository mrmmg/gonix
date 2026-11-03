package realip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/nginxtest"
	"github.com/mrmmg/gonix/internal/snippet"
)

func TestRenderParseRoundTrip(t *testing.T) {
	for _, s := range []Settings{
		{Cloudflare: true, Header: "CF-Connecting-IP", Proxies: CloudflareRanges},
		{Header: "X-Forwarded-For", Proxies: []string{"10.0.0.0/8", "192.168.1.10"}},
	} {
		out, err := Render(s)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if got := Parse(out); !reflect.DeepEqual(got, s) {
			t.Errorf("round trip = %+v, want %+v", got, s)
		}
		if !strings.Contains(out, "real_ip_recursive on;") {
			t.Errorf("missing real_ip_recursive:\n%s", out)
		}
	}
}

func TestValidate(t *testing.T) {
	for _, s := range []Settings{
		{Header: "X-Evil; deny all", Proxies: []string{"10.0.0.1"}},
		{Header: "X-Forwarded-For"},
		{Header: "X-Forwarded-For", Proxies: []string{"10.0.0.0/40"}},
	} {
		if err := s.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want error", s)
		}
	}
}

func TestFetchCloudflareRanges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v4" {
			w.Write([]byte("173.245.48.0/20\n103.21.244.0/22\n"))
			return
		}
		w.Write([]byte("2400:cb00::/32\n"))
	}))
	defer srv.Close()

	old := CloudflareURLs
	defer func() { CloudflareURLs = old }()
	CloudflareURLs = []string{srv.URL + "/v4", srv.URL + "/v6"}

	got, err := FetchCloudflareRanges(context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("FetchCloudflareRanges: %v", err)
	}
	want := []string{"173.245.48.0/20", "103.21.244.0/22", "2400:cb00::/32"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ranges = %v, want %v", got, want)
	}
}

func TestFetchCloudflareRangesRejectsGarbage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("173.245.48.0/20;\ninclude /etc/passwd\n"))
	}))
	defer srv.Close()
	old := CloudflareURLs
	defer func() { CloudflareURLs = old }()
	CloudflareURLs = []string{srv.URL}

	if _, err := FetchCloudflareRanges(context.Background(), srv.Client()); err == nil {
		t.Error("expected malformed ranges to be rejected")
	}
}

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func TestServiceSaveGetDisable(t *testing.T) {
	svc := &Service{Dir: t.TempDir(), Writer: &snippet.Writer{Tester: okTester{}}}
	if _, enabled, err := svc.Get(); err != nil || enabled {
		t.Fatalf("Get on fresh install = enabled %v, err %v", enabled, err)
	}
	s := Settings{Header: "X-Real-IP", Proxies: []string{"10.0.0.1"}}
	if err := svc.Save(context.Background(), s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, enabled, err := svc.Get()
	if err != nil || !enabled || !reflect.DeepEqual(got, s) {
		t.Fatalf("Get = %+v, %v, %v", got, enabled, err)
	}
	if err := svc.Disable(context.Background()); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if _, enabled, _ := svc.Get(); enabled {
		t.Error("expected real IP to be disabled")
	}
}

func TestPassesRealNginx(t *testing.T) {
	p := nginxtest.New(t)
	out, err := Render(Settings{Cloudflare: true, Header: "CF-Connecting-IP", Proxies: CloudflareRanges})
	if err != nil {
		t.Fatal(err)
	}
	p.AddHTTP(t, FileName, out)
	p.Test(t)
}
