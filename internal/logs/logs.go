// Package logs provides access/error log path helpers and generates the
// logrotate configuration used to rotate both Nginx's own logs and
// GoNix's audit log.
package logs

import (
	"fmt"
	"path/filepath"
)

// AccessLogPath returns the access log path for a host, matching what
// internal/nginx's Renderer writes into the generated configuration.
func AccessLogPath(nginxLogDir, serverName string) string {
	return filepath.Join(nginxLogDir, serverName+".access.log")
}

// ErrorLogPath returns the error log path for a host.
func ErrorLogPath(nginxLogDir, serverName string) string {
	return filepath.Join(nginxLogDir, serverName+".error.log")
}

// LogrotateConfig renders a logrotate configuration snippet covering both
// per-host Nginx logs and the GoNix audit log, matching configs/logrotate.conf
// which the installers place at /etc/logrotate.d/gonix.
func LogrotateConfig(nginxLogDir, auditLogFile string) string {
	return fmt.Sprintf(`%s/*.access.log %s/*.error.log {
    daily
    rotate 14
    missingok
    notifempty
    compress
    delaycompress
    sharedscripts
    postrotate
        [ -f /var/run/nginx.pid ] && kill -USR1 $(cat /var/run/nginx.pid)
    endscript
}

%s {
    weekly
    rotate 12
    missingok
    notifempty
    compress
    delaycompress
}
`, nginxLogDir, nginxLogDir, auditLogFile)
}
