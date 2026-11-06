package metrics

import "github.com/prometheus/client_golang/prometheus"

// sample is one series, before it becomes a Prometheus metric. Collectors
// return samples (rather than metrics) so slow results can be cached and
// each scrape can still filter them by host.
type sample struct {
	desc   *prometheus.Desc
	typ    prometheus.ValueType
	val    float64
	labels []string
	// names are the host names the series is about; empty means it is not
	// about one host and is never filtered out.
	names []string
}

func (s sample) metric() prometheus.Metric {
	return prometheus.MustNewConstMetric(s.desc, s.typ, s.val, s.labels...)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
