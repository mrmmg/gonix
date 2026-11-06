package metrics

import (
	"context"
	"regexp"
	"strconv"
	"sync"

	"github.com/mrmmg/gonix/internal/logs"
)

var reErrorLevel = regexp.MustCompile(`\[(debug|info|notice|warn|error|crit|alert|emerg)\]`)

// trafficCollector counts the requests in every host's access log by
// status code, plus the bytes sent and the entries of its error log.
// Counts start at zero when the exporter starts; Prometheus' rate() copes
// with that like with any restart.
func (e *Exporter) trafficCollector() func(context.Context) ([]sample, error) {
	requests := e.desc(MetricHTTPRequestsTotal)
	sent := e.desc(MetricHTTPResponseBytesTotal)
	errlog := e.desc(MetricErrorLogEntriesTotal)

	var mu sync.Mutex
	tail := newTailer()
	e.closers = append(e.closers, tail.close)
	reqCount := map[[2]string]float64{}
	byteCount := map[string]float64{}
	errCount := map[[2]string]float64{}
	aliases := map[string][]string{}

	return func(context.Context) ([]sample, error) {
		list, err := e.hosts()
		if err != nil {
			return nil, err
		}
		mu.Lock()
		defer mu.Unlock()

		paths := map[string]bool{}
		for _, h := range list {
			if !e.opt.Filter.Match(hostNames(h.Host)...) {
				continue
			}
			host := h.ServerName
			aliases[host] = hostNames(h.Host)
			access, errPath := logs.AccessLogPath(e.src.LogDir, host), logs.ErrorLogPath(e.src.LogDir, host)
			paths[access], paths[errPath] = true, true
			tail.poll(access, func(line string) {
				if en, ok := logs.ParseAccessLine(line); ok && en.Status >= 100 && en.Status <= 599 {
					reqCount[[2]string{host, strconv.Itoa(en.Status)}]++
					byteCount[host] += float64(en.Bytes)
				}
			})
			tail.poll(errPath, func(line string) {
				if m := reErrorLevel.FindStringSubmatch(line); m != nil {
					errCount[[2]string{host, m[1]}]++
				}
			})
		}
		tail.keep(paths)
		tail.prime()

		var s []sample
		for k, v := range reqCount {
			s = append(s, counter(requests, v, aliases[k[0]], k[0], k[1]))
		}
		for host, v := range byteCount {
			s = append(s, counter(sent, v, aliases[host], host))
		}
		for k, v := range errCount {
			s = append(s, counter(errlog, v, aliases[k[0]], k[0], k[1]))
		}
		return s, nil
	}
}
