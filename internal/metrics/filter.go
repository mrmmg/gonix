package metrics

import (
	"path"
	"strings"
)

// Filter selects hosts by shell-style patterns such as "*.example.com".
type Filter struct {
	Include []string // empty matches every host
	Exclude []string
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(strings.ToLower(p), strings.ToLower(name)); ok {
			return true
		}
	}
	return false
}

// Match reports whether a series about the given names (a host and its
// aliases, or a certificate's domains) is kept. No names means the series
// is not about one host, and is always kept.
func (f Filter) Match(names ...string) bool {
	if len(names) == 0 {
		return true
	}
	included := len(f.Include) == 0
	for _, n := range names {
		if matchAny(f.Exclude, n) {
			return false
		}
		if matchAny(f.Include, n) {
			included = true
		}
	}
	return included
}
