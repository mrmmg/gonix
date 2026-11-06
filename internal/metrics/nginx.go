package metrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// nginxCollector reports the service (is it running, does its configuration
// pass nginx -t), its processes, and, when enabled, the connection counters
// of stub_status.
func (e *Exporter) nginxCollector() func(context.Context) ([]sample, error) {
	up := e.desc(MetricNginxUp)
	active := e.desc(MetricNginxServiceActive)
	boot := e.desc(MetricNginxServiceEnabled)
	valid := e.desc(MetricNginxConfigValid)
	version := e.desc(MetricNginxVersionInfo)
	procs := e.desc(MetricNginxProcesses)
	uptime := e.desc(MetricNginxUptimeSeconds)
	rss := e.desc(MetricNginxMemoryResidentBytes)
	conns := e.desc(MetricNginxConnections)
	accepted := e.desc(MetricNginxConnectionsAcceptedTotal)
	handled := e.desc(MetricNginxConnectionsHandledTotal)
	requests := e.desc(MetricNginxHTTPRequestsTotal)
	stubUp := e.desc(MetricNginxStubStatusUp)

	slow := &memo{ttl: e.ttl()}
	return func(ctx context.Context) ([]sample, error) {
		var firstErr error
		note := func(err error) {
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
		s, err := slow.get(func() ([]sample, error) {
			var s []sample
			st, err := e.src.NginxStatus(ctx)
			if err != nil {
				return nil, err
			}
			s = append(s, gauge(active, b2f(st.Active), nil), gauge(boot, b2f(st.Enabled), nil))
			ok, err := e.src.NginxTest(ctx)
			if err != nil {
				return s, err
			}
			s = append(s, gauge(valid, b2f(ok), nil))
			if e.src.NginxVersion != "" {
				s = append(s, gauge(version, 1, nil, e.src.NginxVersion))
			}
			return s, nil
		})
		note(err)
		s = append([]sample{}, s...)

		// Processes are read from /proc on every scrape: a crash must show up at once.
		info, err := e.src.Processes(ctx)
		note(err)
		s = append(s, gauge(up, b2f(info.MasterPID != 0), nil),
			gauge(procs, b2f(info.MasterPID != 0), nil, "master"),
			gauge(procs, float64(info.WorkerCount), nil, "worker"))
		if info.MasterPID != 0 {
			if d, err := e.src.Uptime(info.MasterPID); err == nil {
				s = append(s, gauge(uptime, d.Seconds(), nil))
			}
			var total uint64
			for _, pid := range append([]int{info.MasterPID}, info.WorkerPIDs...) {
				if b, err := e.src.RSS(pid); err == nil {
					total += b
				}
			}
			s = append(s, gauge(rss, float64(total), nil))
		}

		if e.src.StubStatus != nil {
			body, err := e.src.StubStatus(ctx)
			st, perr := parseStubStatus(body)
			if err == nil {
				err = perr
			}
			s = append(s, gauge(stubUp, b2f(err == nil), nil))
			if err != nil {
				note(fmt.Errorf("stub_status: %w", err))
			} else {
				for _, c := range []struct {
					state string
					v     float64
				}{{"active", st.active}, {"reading", st.reading}, {"writing", st.writing}, {"waiting", st.waiting}} {
					s = append(s, gauge(conns, c.v, nil, c.state))
				}
				s = append(s, counter(accepted, st.accepted, nil), counter(handled, st.handled, nil), counter(requests, st.requests, nil))
			}
		}
		return s, firstErr
	}
}

type stubStatus struct{ active, accepted, handled, requests, reading, writing, waiting float64 }

// parseStubStatus reads the page of Nginx's stub_status module:
//
//	Active connections: 291
//	server accepts handled requests
//	 16630948 16630948 31070465
//	Reading: 6 Writing: 179 Waiting: 106
func parseStubStatus(body string) (stubStatus, error) {
	var st stubStatus
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) < 4 || !strings.HasPrefix(lines[0], "Active connections:") {
		return st, fmt.Errorf("unexpected stub_status output")
	}
	num := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	st.active = num(strings.TrimSpace(strings.TrimPrefix(lines[0], "Active connections:")))
	f := strings.Fields(lines[2])
	if len(f) != 3 {
		return st, fmt.Errorf("unexpected stub_status counters")
	}
	st.accepted, st.handled, st.requests = num(f[0]), num(f[1]), num(f[2])
	g := strings.Fields(lines[3]) // Reading: 6 Writing: 179 Waiting: 106
	if len(g) != 6 {
		return st, fmt.Errorf("unexpected stub_status states")
	}
	st.reading, st.writing, st.waiting = num(g[1]), num(g[3]), num(g[5])
	return st, nil
}
