package health

import (
	"context"
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mrmmg/gonix/internal/nginx"
)

func TestUpstreams(t *testing.T) {
	proxy := func(host string, port int) *nginx.ProxyConfig {
		return &nginx.ProxyConfig{UpstreamScheme: "http", UpstreamHost: host, UpstreamPort: port}
	}
	h := nginx.Host{Locations: []nginx.Location{
		{Path: "/", Proxy: proxy("127.0.0.1", 8080)},
		{Path: "/api/", Proxy: proxy("127.0.0.1", 8080)},
		{Path: "/v6/", Proxy: proxy("::1", 9000)},
		{Path: "/c/", IsCustom: true},
	}}
	if got, want := Upstreams(h), []string{"127.0.0.1:8080", "[::1]:9000"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Upstreams = %v, want %v", got, want)
	}
}

func TestCheck(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	up := l.Addr().String()

	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	down := "127.0.0.1:" + strconv.Itoa(closed.Addr().(*net.TCPAddr).Port)
	closed.Close()

	failed := Check(context.Background(), []string{up, down}, time.Second)
	if _, ok := failed[up]; ok {
		t.Errorf("%s reported down: %v", up, failed[up])
	}
	if _, ok := failed[down]; !ok {
		t.Errorf("%s should be reported down", down)
	}
}
