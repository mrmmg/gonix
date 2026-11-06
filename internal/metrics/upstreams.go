package metrics

import (
	"context"
	"sort"
	"time"

	"github.com/mrmmg/gonix/internal/health"
)

const probeTimeout = 2 * time.Second

// upstreamsCollector dials the backends of every reverse-proxy host,
// including the servers of the upstream groups they use.
func (e *Exporter) upstreamsCollector() func(context.Context) ([]sample, error) {
	up := e.desc(MetricUpstreamUp)
	connect := e.desc(MetricUpstreamConnectSeconds)
	m := &memo{ttl: e.ttl()}

	type target struct{ host, group, addr string }
	return func(ctx context.Context) ([]sample, error) {
		return m.get(func() ([]sample, error) {
			list, err := e.hosts()
			if err != nil {
				return nil, err
			}
			groups := map[string][]string{}
			if e.src.Groups != nil {
				if gs, err := e.src.Groups(); err == nil {
					for _, g := range gs {
						for _, s := range g.Servers {
							if !s.Backup {
								groups[g.Name] = append(groups[g.Name], s.Address)
							}
						}
					}
				}
			}
			var targets []target
			seen := map[target]bool{}
			add := func(t target) {
				if !seen[t] {
					seen[t] = true
					targets = append(targets, t)
				}
			}
			names := map[string][]string{}
			for _, h := range list {
				names[h.ServerName] = hostNames(h.Host)
				for _, a := range health.Upstreams(h.Host) {
					add(target{h.ServerName, "", a})
				}
				for _, loc := range h.Locations {
					if loc.Proxy == nil || loc.Proxy.UpstreamGroup == "" {
						continue
					}
					for _, a := range groups[loc.Proxy.UpstreamGroup] {
						add(target{h.ServerName, loc.Proxy.UpstreamGroup, a})
					}
				}
			}
			addrSet := map[string]bool{}
			for _, t := range targets {
				addrSet[t.addr] = true
			}
			var addrs []string
			for a := range addrSet {
				addrs = append(addrs, a)
			}
			sort.Strings(addrs)
			probes := e.src.Probe(ctx, addrs)

			var s []sample
			for _, t := range targets {
				p := probes[t.addr]
				s = append(s, gauge(up, b2f(p.Err == nil), names[t.host], t.host, t.group, t.addr))
				if p.Err == nil {
					s = append(s, gauge(connect, p.Duration.Seconds(), names[t.host], t.host, t.group, t.addr))
				}
			}
			return s, nil
		})
	}
}
