package upstreams

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/nginxtest"
	"github.com/mrmmg/gonix/internal/snippet"
)

func TestParseFormatServers(t *testing.T) {
	in := "10.0.0.11:3000, 10.0.0.12:3000 weight=2 , [::1]:3000 backup"
	servers, err := ParseServers(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []Server{{Address: "10.0.0.11:3000"}, {Address: "10.0.0.12:3000", Weight: 2}, {Address: "[::1]:3000", Backup: true}}
	if !reflect.DeepEqual(servers, want) {
		t.Errorf("ParseServers = %+v", servers)
	}
	if again, _ := ParseServers(FormatServers(servers)); !reflect.DeepEqual(again, servers) {
		t.Errorf("Format round trip = %+v", again)
	}
	for _, bad := range []string{"", "10.0.0.1", "10.0.0.1:99999", "10.0.0.1:80 weight=0", "10.0.0.1:80 down", "a;b:80"} {
		if _, err := ParseServers(bad); err == nil {
			t.Errorf("ParseServers(%q) should fail", bad)
		}
	}
}

func TestRenderParseRoundTrip(t *testing.T) {
	g := Group{Name: "app", Method: LeastConn, MaxFails: 3, FailTimeout: "10s", Servers: []Server{
		{Address: "10.0.0.11:3000"}, {Address: "app2.internal:3000", Weight: 3}, {Address: "10.0.0.13:3000", Backup: true},
	}}
	out, err := Render(g)
	if err != nil {
		t.Fatal(err)
	}
	if got := Parse("app", out); !reflect.DeepEqual(got, g) {
		t.Errorf("round trip = %+v, want %+v\n%s", got, g, out)
	}
}

func TestValidate(t *testing.T) {
	base := func() Group {
		return Group{Name: "app", Method: RoundRobin, MaxFails: 1, Servers: []Server{{Address: "10.0.0.1:80"}}}
	}
	for name, mutate := range map[string]func(*Group){
		"name":       func(g *Group) { g.Name = "a b" },
		"method":     func(g *Group) { g.Method = "random" },
		"no servers": func(g *Group) { g.Servers = nil },
		"only backup": func(g *Group) {
			g.Servers = []Server{{Address: "10.0.0.1:80", Backup: true}}
		},
		"ip_hash backup": func(g *Group) {
			g.Method = IPHash
			g.Servers = append(g.Servers, Server{Address: "10.0.0.2:80", Backup: true})
		},
		"timeout": func(g *Group) { g.FailTimeout = "10 s" },
	} {
		g := base()
		mutate(&g)
		if err := g.Validate(); err == nil {
			t.Errorf("%s: Validate = nil, want error", name)
		}
	}
}

type okTester struct{}

func (okTester) Test(context.Context) (nginx.Result, error) { return nginx.Result{OK: true}, nil }

func TestServiceLifecycleAndProxyRoundTrip(t *testing.T) {
	r, err := nginx.NewRenderer(t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mgr := nginx.NewManager(t.TempDir(), t.TempDir(), r)
	svc := &Service{Dir: t.TempDir(), Manager: mgr, Writer: &snippet.Writer{Tester: okTester{}}}
	ctx := context.Background()
	g := Group{Name: "app", Method: RoundRobin, MaxFails: 3, FailTimeout: "10s", Servers: []Server{{Address: "10.0.0.1:80"}}}
	if err := svc.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	if groups, _ := svc.List(); len(groups) != 1 || !reflect.DeepEqual(groups[0], g) {
		t.Errorf("List = %+v", groups)
	}

	h := nginx.Host{ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80, Locations: []nginx.Location{
		{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamGroup: "app", HTTPVersion: "1.1"}},
	}}
	if err := mgr.WriteHost(h); err != nil {
		t.Fatal(err)
	}
	parsed, err := mgr.GetHost("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if p := parsed.Locations[0].Proxy; p.UpstreamGroup != "app" || p.UpstreamHost != "" {
		t.Errorf("parsed proxy = %+v", p)
	}
	if err := svc.Delete(ctx, "app"); err == nil {
		t.Error("expected Delete to refuse a group in use")
	}
	os.Remove(filepath.Join(mgr.SitesAvailable, "example.com"))
	if err := svc.Delete(ctx, "app"); err != nil {
		t.Fatal(err)
	}
}

// TestLoadBalancingBehavior proxies through a real Nginx to a group whose
// first server is down, checking the request still reaches the live one.
func TestLoadBalancingBehavior(t *testing.T) {
	p := nginxtest.New(t)
	backend := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "from backend")
	})}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go backend.Serve(l)
	t.Cleanup(func() { backend.Close() })
	dead, _ := net.Listen("tcp", "127.0.0.1:0")
	deadAddr := dead.Addr().String()
	dead.Close()

	group, err := Render(Group{Name: "app", Method: RoundRobin, MaxFails: 1, FailTimeout: "30s", Servers: []Server{
		{Address: deadAddr}, {Address: l.Addr().String()},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p.AddHTTP(t, nginx.UpstreamFileName("app"), group)
	r, err := nginx.NewRenderer(p.Dir, p.Dir, p.Dir, p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := r.Render(nginx.Host{ServerName: "example.com", Mode: nginx.ModeReverseProxy, Listen: 80, Locations: []nginx.Location{
		{Path: "/", Proxy: &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamGroup: "app", HTTPVersion: "1.1", ForwardHost: true}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p.AddSite(t, "example.com", site)
	p.Start(t)

	for i := 0; i < 4; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(p.Port(t, 80))+"/", nil)
		req.Host = "example.com"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(body) != "from backend" {
			t.Errorf("request %d = %d %q", i, resp.StatusCode, body)
		}
	}
}
