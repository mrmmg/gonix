package logs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry is one parsed access log line in Nginx's default "combined" format:
//
//	$remote_addr - $remote_user [$time_local] "$request" $status
//	$body_bytes_sent "$http_referer" "$http_user_agent"
type Entry struct {
	Raw       string
	IP        string
	Time      time.Time
	Method    string
	Path      string
	Status    int
	Bytes     int64
	Referer   string
	UserAgent string
}

var reStatusFilter = regexp.MustCompile(`^(\d{3}|[1-5]xx)$`)

var reCombined = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "([^"]*)" (\d{3}) (\d+|-)(?: "([^"]*)" "([^"]*)")?`)

// ParseAccessLine parses one combined-format line.
func ParseAccessLine(line string) (Entry, bool) {
	m := reCombined.FindStringSubmatch(line)
	if m == nil {
		return Entry{}, false
	}
	e := Entry{Raw: line, IP: m[1], Referer: m[6], UserAgent: m[7]}
	e.Time, _ = time.Parse("02/Jan/2006:15:04:05 -0700", m[2])
	if parts := strings.Fields(m[3]); len(parts) >= 2 {
		e.Method, e.Path = parts[0], parts[1]
	} else {
		e.Path = m[3]
	}
	e.Status, _ = strconv.Atoi(m[4])
	e.Bytes, _ = strconv.ParseInt(m[5], 10, 64)
	return e, true
}

// Tail returns up to n last lines of the file at path, reading backwards
// from the end so large logs stay cheap.
func Tail(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	const chunk = 64 << 10
	size := info.Size()
	var buf []byte
	for offset := size; offset > 0 && bytes.Count(buf, []byte("\n")) <= n; {
		read := int64(chunk)
		if offset < read {
			read = offset
		}
		offset -= read
		part := make([]byte, read)
		if _, err := f.ReadAt(part, offset); err != nil && err != io.EOF {
			return nil, err
		}
		buf = append(part, buf...)
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// Filter selects entries. Empty fields match everything.
type Filter struct {
	Status string // "5xx", "4xx", or an exact code such as "404"
	IP     string // exact client address
	Path   string // substring of the request path
}

// ParseFilter parses "status:5xx ip:1.2.3.4 path:/api"; a bare word is a
// path substring.
func ParseFilter(s string) (Filter, error) {
	var f Filter
	for _, tok := range strings.Fields(s) {
		key, val, ok := strings.Cut(tok, ":")
		if !ok {
			f.Path = tok
			continue
		}
		switch key {
		case "status":
			if !reStatusFilter.MatchString(val) {
				return Filter{}, fmt.Errorf("status must be a code (404) or a class (5xx)")
			}
			f.Status = val
		case "ip":
			f.IP = val
		case "path":
			f.Path = val
		default:
			return Filter{}, fmt.Errorf("unknown filter %q (use status:, ip: or path:)", key)
		}
	}
	return f, nil
}

// String is the inverse of ParseFilter.
func (f Filter) String() string {
	var parts []string
	if f.Status != "" {
		parts = append(parts, "status:"+f.Status)
	}
	if f.IP != "" {
		parts = append(parts, "ip:"+f.IP)
	}
	if f.Path != "" {
		parts = append(parts, "path:"+f.Path)
	}
	return strings.Join(parts, " ")
}

// Match reports whether e passes the filter.
func (f Filter) Match(e Entry) bool {
	if f.IP != "" && e.IP != f.IP {
		return false
	}
	if f.Path != "" && !strings.Contains(e.Path, f.Path) {
		return false
	}
	if f.Status != "" {
		code := strconv.Itoa(e.Status)
		if strings.HasSuffix(f.Status, "xx") {
			return code[0] == f.Status[0]
		}
		return code == f.Status
	}
	return true
}

// Count is a value with its number of occurrences.
type Count struct {
	Value string
	N     int
}

// Summary aggregates access log entries.
type Summary struct {
	Total     int
	ByClass   map[string]int // "2xx", "3xx", "4xx", "5xx"
	TopIPs    []Count
	TopPaths  []Count
	Top5xx    []Count // paths answered with 5xx
	PerMinute float64
}

// Summarize aggregates entries, keeping the top n of each ranking.
func Summarize(entries []Entry, n int) Summary {
	s := Summary{ByClass: map[string]int{}}
	ips, paths, errs := map[string]int{}, map[string]int{}, map[string]int{}
	var first, last time.Time
	for _, e := range entries {
		s.Total++
		s.ByClass[strconv.Itoa(e.Status/100)+"xx"]++
		ips[e.IP]++
		paths[e.Path]++
		if e.Status >= 500 {
			errs[e.Path]++
		}
		if !e.Time.IsZero() {
			if first.IsZero() || e.Time.Before(first) {
				first = e.Time
			}
			if e.Time.After(last) {
				last = e.Time
			}
		}
	}
	if minutes := last.Sub(first).Minutes(); minutes >= 1 {
		s.PerMinute = float64(s.Total) / minutes
	}
	s.TopIPs, s.TopPaths, s.Top5xx = top(ips, n), top(paths, n), top(errs, n)
	return s
}

func top(m map[string]int, n int) []Count {
	out := make([]Count, 0, len(m))
	for v, c := range m {
		out = append(out, Count{v, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Value < out[j].Value
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
