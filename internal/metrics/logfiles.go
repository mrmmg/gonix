package metrics

import (
	"context"
	"os"
	"syscall"

	"github.com/mrmmg/gonix/internal/logs"
)

// logfilesCollector reports how big each host's logs are, when they were
// last written, and how much room is left on the disk holding them.
func (e *Exporter) logfilesCollector() func(context.Context) ([]sample, error) {
	size := e.desc(MetricLogSizeBytes)
	written := e.desc(MetricLogLastWriteTimestampSeconds)
	free := e.desc(MetricLogDirFreeBytes)
	total := e.desc(MetricLogDirTotalBytes)

	return func(context.Context) ([]sample, error) {
		list, err := e.hosts()
		if err != nil {
			return nil, err
		}
		var s []sample
		seen := map[string]bool{}
		for _, h := range list {
			if seen[h.ServerName] {
				continue
			}
			seen[h.ServerName] = true
			names := hostNames(h.Host)
			for _, f := range []struct{ typ, path string }{
				{"access", logs.AccessLogPath(e.src.LogDir, h.ServerName)},
				{"error", logs.ErrorLogPath(e.src.LogDir, h.ServerName)},
			} {
				fi, err := os.Stat(f.path)
				if err != nil {
					continue
				}
				s = append(s, gauge(size, float64(fi.Size()), names, h.ServerName, f.typ))
				if f.typ == "access" {
					s = append(s, gauge(written, float64(fi.ModTime().Unix()), names, h.ServerName))
				}
			}
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(e.src.LogDir, &st); err != nil {
			return s, err
		}
		s = append(s, gauge(free, float64(st.Bavail)*float64(st.Bsize), nil), gauge(total, float64(st.Blocks)*float64(st.Bsize), nil))
		return s, nil
	}
}
