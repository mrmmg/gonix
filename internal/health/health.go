// Package health checks whether hosts' reverse-proxy upstreams accept
// connections, so a dead backend shows up in the UI before users report
// 502 errors.
package health

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/mrmmg/gonix/internal/nginx"
)

// Upstreams returns the distinct host:port addresses h proxies to.
func Upstreams(h nginx.Host) []string {
	seen := map[string]bool{}
	var out []string
	for _, loc := range h.Locations {
		if loc.Proxy == nil || loc.Proxy.UpstreamHost == "" {
			continue
		}
		addr := net.JoinHostPort(loc.Proxy.UpstreamHost, strconv.Itoa(loc.Proxy.UpstreamPort))
		if !seen[addr] {
			seen[addr] = true
			out = append(out, addr)
		}
	}
	return out
}

// Probe is the outcome of dialing one address.
type Probe struct {
	Err      error
	Duration time.Duration // time to connect, or until the attempt failed
}

// Probes dials every address concurrently and returns the outcome for each.
func Probes(ctx context.Context, addrs []string, timeout time.Duration) map[string]Probe {
	out := map[string]Probe{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	d := net.Dialer{Timeout: timeout}
	for _, addr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			start := time.Now()
			c, err := d.DialContext(ctx, "tcp", addr)
			if err == nil {
				c.Close()
			}
			mu.Lock()
			out[addr] = Probe{Err: err, Duration: time.Since(start)}
			mu.Unlock()
		}(addr)
	}
	wg.Wait()
	return out
}

// Check dials every address concurrently and returns the error for each
// address that did not accept a TCP connection within timeout.
func Check(ctx context.Context, addrs []string, timeout time.Duration) map[string]error {
	failed := map[string]error{}
	for addr, p := range Probes(ctx, addrs, timeout) {
		if p.Err != nil {
			failed[addr] = p.Err
		}
	}
	return failed
}
