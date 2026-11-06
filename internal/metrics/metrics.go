// Package metrics serves GoNix's Prometheus exporter: what GoNix knows
// about Nginx and the hosts it manages (certificates, upstreams, hosts,
// the service, traffic counted from the access logs), as structured
// numbers for Prometheus and Grafana.
//
// Metrics are grouped in collectors that can be switched on and off, and
// every per-host series can be filtered by host, both in the configuration
// and per scrape (?host=, ?exclude_host=, ?collect[]=).
package metrics

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/health"
	"github.com/mrmmg/gonix/internal/lint"
	"github.com/mrmmg/gonix/internal/nginx"
	"github.com/mrmmg/gonix/internal/system"
	"github.com/mrmmg/gonix/internal/upstreams"
)

// Options are the exporter's settings.
type Options struct {
	Instance string // names the Nginx instance; "default" when empty
	Version  string // GoNix version, exported as gonix_build_info
	// Filter limits per-host series, in addition to any per-scrape filter.
	Filter Filter
	// Collectors lists the enabled collectors; empty enables all.
	Collectors []string
	// CacheTTL is how long slow results are reused.
	CacheTTL time.Duration
}

// Sources is where metrics are read from. The fields are functions so tests
// can fake them; FromServices fills them from a real installation.
type Sources struct {
	Hosts        func() ([]nginx.ListedHost, error)
	Certs        func() []certificates.Certificate
	Lint         func(ctx context.Context) ([]lint.Finding, error)
	Groups       func() ([]upstreams.Group, error)
	Probe        func(ctx context.Context, addrs []string) map[string]health.Probe
	NginxStatus  func(ctx context.Context) (system.Status, error)
	NginxTest    func(ctx context.Context) (bool, error)
	NginxVersion string
	Processes    func(ctx context.Context) (system.ProcessInfo, error)
	RSS          func(pid int) (uint64, error)
	Uptime       func(pid int) (time.Duration, error)
	// StubStatus returns the stub_status page; nil when it is not enabled.
	StubStatus func(ctx context.Context) (string, error)
	LogDir     string
	AuditFile  string
}

type collector struct {
	name  string
	fetch func(ctx context.Context) ([]sample, error)
}

// Exporter holds the collectors and their state (cached results, log
// positions, counters).
type Exporter struct {
	opt        Options
	src        Sources
	labels     prometheus.Labels
	collectors []collector

	hostsMu  sync.Mutex
	hostsAt  time.Time
	hostsVal []nginx.ListedHost
	hostsErr error

	durationDesc, successDesc, buildDesc *prometheus.Desc
	closers                              []func()
}

// New builds the exporter. Call Close when done.
func New(opt Options, src Sources) *Exporter {
	if opt.Instance == "" {
		opt.Instance = "default"
	}
	e := &Exporter{opt: opt, src: src, labels: prometheus.Labels{LabelInstance: opt.Instance}}
	e.durationDesc = e.desc(MetricExporterCollectorDurationSeconds)
	e.successDesc = e.desc(MetricExporterCollectorSuccess)
	e.buildDesc = e.desc(MetricBuildInfo)

	all := []collector{
		{"gonix", e.gonixCollector()},
		{"certificates", e.certificatesCollector()},
		{"upstreams", e.upstreamsCollector()},
		{"nginx", e.nginxCollector()},
		{"traffic", e.trafficCollector()},
		{"logfiles", e.logfilesCollector()},
	}
	for _, c := range all {
		if len(opt.Collectors) == 0 || slices.Contains(opt.Collectors, c.name) {
			e.collectors = append(e.collectors, c)
		}
	}
	return e
}

// Close releases the open log files.
func (e *Exporter) Close() {
	for _, c := range e.closers {
		c()
	}
}

func (e *Exporter) ttl() time.Duration { return e.opt.CacheTTL }

// desc builds the descriptor of a metric from its definition in defs.go.
func (e *Exporter) desc(name string) *prometheus.Desc {
	d, ok := Lookup(name)
	if !ok {
		panic("metrics: no definition for " + name)
	}
	return prometheus.NewDesc(d.Name, d.Help, d.Labels, e.labels)
}

func gauge(d *prometheus.Desc, v float64, names []string, labels ...string) sample {
	return sample{desc: d, typ: prometheus.GaugeValue, val: v, labels: labels, names: names}
}

func counter(d *prometheus.Desc, v float64, names []string, labels ...string) sample {
	return sample{desc: d, typ: prometheus.CounterValue, val: v, labels: labels, names: names}
}

// hosts lists the hosts, reusing the result for the cache TTL.
func (e *Exporter) hosts() ([]nginx.ListedHost, error) {
	e.hostsMu.Lock()
	defer e.hostsMu.Unlock()
	if e.hostsAt.IsZero() || time.Since(e.hostsAt) >= e.ttl() {
		e.hostsVal, e.hostsErr = e.src.Hosts()
		e.hostsAt = time.Now()
	}
	return e.hostsVal, e.hostsErr
}

// hostNames returns a host's name and aliases.
func hostNames(h nginx.Host) []string {
	return append([]string{h.ServerName}, h.ServerAlias...)
}

// request is the collector the registry of one scrape gathers from.
type request struct {
	e       *Exporter
	ctx     context.Context
	filters []Filter
	only    []string
}

func (r *request) Describe(chan<- *prometheus.Desc) {} // unchecked: the series depend on the installation

func (r *request) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(r.e.buildDesc, prometheus.GaugeValue, 1, r.e.opt.Version)
	var wg sync.WaitGroup
	for _, c := range r.e.collectors {
		if len(r.only) > 0 && !slices.Contains(r.only, c.name) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			samples, err := c.fetch(r.ctx)
			ch <- prometheus.MustNewConstMetric(r.e.durationDesc, prometheus.GaugeValue, time.Since(start).Seconds(), c.name)
			ch <- prometheus.MustNewConstMetric(r.e.successDesc, prometheus.GaugeValue, b2f(err == nil), c.name)
			for _, s := range samples {
				if r.keep(s) {
					ch <- s.metric()
				}
			}
		}()
	}
	wg.Wait()
}

func (r *request) keep(s sample) bool {
	for _, f := range r.filters {
		if !f.Match(s.names...) {
			return false
		}
	}
	return true
}

// Registry returns a registry for one scrape. only restricts it to some
// collectors, and f to some hosts, on top of the configured filter.
func (e *Exporter) Registry(ctx context.Context, f Filter, only []string) (*prometheus.Registry, error) {
	for _, n := range only {
		if !slices.Contains(config.Collectors, n) {
			return nil, fmt.Errorf("unknown collector %q (use %v)", n, config.Collectors)
		}
	}
	reg := prometheus.NewRegistry()
	if err := reg.Register(&request{e: e, ctx: ctx, filters: []Filter{e.opt.Filter, f}, only: only}); err != nil {
		return nil, err
	}
	return reg, nil
}

// Write prints one scrape in the Prometheus text format.
func (e *Exporter) Write(ctx context.Context, w io.Writer, f Filter, only []string) error {
	reg, err := e.Registry(ctx, f, only)
	if err != nil {
		return err
	}
	families, err := reg.Gather()
	if err != nil {
		return err
	}
	enc := expfmt.NewEncoder(w, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range families {
		if err := enc.Encode(mf); err != nil {
			return err
		}
	}
	return nil
}
