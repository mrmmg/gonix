package metrics

import (
	"context"
	"time"

	"github.com/mrmmg/gonix/internal/certificates"
)

// certificatesCollector reports every certificate GoNix can see: how long
// each has left, whether it is due for renewal, and whether it is sound.
func (e *Exporter) certificatesCollector() func(context.Context) ([]sample, error) {
	notAfter := e.desc(MetricCertificateNotAfterTimestampSeconds)
	remaining := e.desc(MetricCertificateRemainingSeconds)
	due := e.desc(MetricCertificateDueForRenewal)
	valid := e.desc(MetricCertificateValid)
	keyOK := e.desc(MetricCertificateKeyMatches)
	chainOK := e.desc(MetricCertificateChainValid)
	status := e.desc(MetricCertificateStatus)
	count := e.desc(MetricCertificates)
	m := &memo{ttl: e.ttl()}

	return func(ctx context.Context) ([]sample, error) {
		return m.get(func() ([]sample, error) {
			var s []sample
			byStatus := map[string]float64{}
			for _, st := range []certificates.Status{certificates.StatusValid, certificates.StatusExpiringSoon, certificates.StatusExpired, certificates.StatusInvalid} {
				byStatus[string(st)] = 0
			}
			seen := map[[2]string]bool{}
			for _, c := range e.src.Certs() {
				key := [2]string{c.Domain, c.Source}
				if seen[key] {
					continue
				}
				seen[key] = true
				byStatus[string(c.Status)]++
				names := append([]string{c.Domain}, c.SANs...)
				l := []string{c.Domain, c.Source}
				ok := c.ParseError == "" && c.Status != certificates.StatusExpired && c.Status != certificates.StatusInvalid
				s = append(s,
					gauge(valid, b2f(ok), names, l...),
					gauge(status, 1, names, c.Domain, c.Source, string(c.Status)))
				if c.ParseError != "" {
					continue
				}
				left := time.Until(c.NotAfter)
				s = append(s,
					gauge(notAfter, float64(c.NotAfter.Unix()), names, l...),
					gauge(remaining, left.Seconds(), names, l...),
					gauge(due, b2f(c.DueForRenewal()), names, l...),
					gauge(keyOK, b2f(c.KeyMatches), names, l...),
					gauge(chainOK, b2f(certificates.VerifyChain(c.CertPath) == nil), names, l...))
			}
			for st, v := range byStatus {
				s = append(s, gauge(count, v, nil, st))
			}
			return s, nil
		})
	}
}
