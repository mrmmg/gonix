package metrics

import (
	"context"
	"strings"
	"sync"

	"github.com/mrmmg/gonix/internal/audit"
	"github.com/mrmmg/gonix/internal/logs"
)

// gonixCollector reports what GoNix manages: the hosts and their state, the
// lint findings and the audited changes.
func (e *Exporter) gonixCollector() func(context.Context) ([]sample, error) {
	hosts := e.desc(MetricHosts)
	info := e.desc(MetricHostInfo)
	enabled := e.desc(MetricHostEnabled)
	maint := e.desc(MetricHostMaintenance)
	lintTotal := e.desc(MetricLintFindings)
	lintTarget := e.desc(MetricLintTargetFindings)
	events := e.desc(MetricAuditEventsTotal)
	last := e.desc(MetricLastChangeTimestampSeconds)

	au := newAuditFollower(e.src.AuditFile)
	e.closers = append(e.closers, au.tail.close)
	m := &memo{ttl: e.ttl()}

	return func(ctx context.Context) ([]sample, error) {
		out, err := m.get(func() ([]sample, error) {
			list, err := e.hosts()
			if err != nil {
				return nil, err
			}
			var s []sample
			n := map[string]float64{"total": float64(len(list)), "enabled": 0, "disabled": 0, "maintenance": 0, "unmanaged": 0, "adopted": 0, "broken_symlink": 0}
			seen := map[string]bool{}
			for _, h := range list {
				switch {
				case h.Enabled:
					n["enabled"]++
				default:
					n["disabled"]++
				}
				if h.Maintenance {
					n["maintenance"]++
				}
				if !h.Managed {
					n["unmanaged"]++
				}
				if h.Adopted != nil {
					n["adopted"]++
				}
				if h.BrokenSymlink {
					n["broken_symlink"]++
				}
				if seen[h.ServerName] {
					continue
				}
				seen[h.ServerName] = true
				names := hostNames(h.Host)
				s = append(s,
					gauge(info, 1, names, h.ServerName, string(h.Mode), boolStr(h.Managed), boolStr(h.SSL.Enabled)),
					gauge(enabled, b2f(h.Enabled), names, h.ServerName),
					gauge(maint, b2f(h.Maintenance), names, h.ServerName))
			}
			for state, v := range n {
				s = append(s, gauge(hosts, v, nil, state))
			}

			if e.src.Lint != nil {
				findings, err := e.src.Lint(ctx)
				if err != nil {
					return s, err
				}
				totals := map[string]float64{"error": 0, "warn": 0, "info": 0}
				perTarget := map[[2]string]float64{}
				for _, f := range findings {
					sev := strings.ToLower(f.Severity.String())
					totals[sev]++
					perTarget[[2]string{f.Target, sev}]++
				}
				for sev, v := range totals {
					s = append(s, gauge(lintTotal, v, nil, sev))
				}
				for k, v := range perTarget {
					s = append(s, gauge(lintTarget, v, []string{k[0]}, k[0], k[1]))
				}
			}
			return s, nil
		})

		au.poll()
		out = append([]sample{}, out...)
		for k, v := range au.counts() {
			out = append(out, counter(events, v, nil, k[0], k[1]))
		}
		for result, t := range au.lastTimes() {
			out = append(out, gauge(last, t, nil, result))
		}
		return out, err
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// auditFollower counts the entries appended to the audit log.
type auditFollower struct {
	path string
	tail *tailer

	mu     sync.Mutex
	count  map[[2]string]float64
	lastTS map[string]float64
}

func newAuditFollower(path string) *auditFollower {
	a := &auditFollower{path: path, tail: newTailer(), count: map[[2]string]float64{}, lastTS: map[string]float64{}}
	if path == "" {
		return a
	}
	// The last change before the exporter started.
	if lines, err := logs.Tail(path, 500); err == nil {
		for _, l := range lines {
			a.noteTime(l)
		}
	}
	a.tail.poll(path, func(string) {}) // follow from the end
	a.tail.prime()
	return a
}

func (a *auditFollower) noteTime(line string) (audit.Entry, bool) {
	en, ok := audit.ParseLine(line)
	if ok {
		a.lastTS[string(en.Result)] = float64(en.Timestamp.Unix())
	}
	return en, ok
}

func (a *auditFollower) poll() {
	if a.path == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tail.poll(a.path, func(line string) {
		if en, ok := a.noteTime(line); ok {
			a.count[[2]string{en.Action, string(en.Result)}]++
		}
	})
}

func (a *auditFollower) counts() map[[2]string]float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[[2]string]float64, len(a.count))
	for k, v := range a.count {
		out[k] = v
	}
	return out
}

func (a *auditFollower) lastTimes() map[string]float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]float64, len(a.lastTS))
	for k, v := range a.lastTS {
		out[k] = v
	}
	return out
}
