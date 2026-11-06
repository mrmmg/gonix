package metrics

import (
	"sync"
	"time"
)

// memo reuses the result of a slow fetch for ttl, so a scrape every few
// seconds does not run nginx -t, dial every backend or parse every
// certificate each time.
type memo struct {
	ttl time.Duration

	mu  sync.Mutex
	at  time.Time
	val []sample
	err error
}

func (m *memo) get(fn func() ([]sample, error)) ([]sample, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.at.IsZero() && time.Since(m.at) < m.ttl {
		return m.val, m.err
	}
	m.val, m.err = fn()
	m.at = time.Now()
	return m.val, m.err
}
