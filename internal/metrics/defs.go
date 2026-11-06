package metrics

import "strings"

// This file is the single source of truth for what the exporter serves: the
// name, type, labels and help of every metric. Collectors build their series
// from these definitions, the dashboard generator (internal/dashboard)
// writes its queries from them, and tests check that what is actually
// scraped matches them. Rename a metric here and the compiler finds every
// use.

// Metric names.
const (
	MetricHosts                               = "gonix_hosts"
	MetricHostInfo                            = "gonix_host_info"
	MetricHostEnabled                         = "gonix_host_enabled"
	MetricHostMaintenance                     = "gonix_host_maintenance"
	MetricLintFindings                        = "gonix_lint_findings"
	MetricLintTargetFindings                  = "gonix_lint_target_findings"
	MetricAuditEventsTotal                    = "gonix_audit_events_total"
	MetricLastChangeTimestampSeconds          = "gonix_last_change_timestamp_seconds"
	MetricCertificateNotAfterTimestampSeconds = "gonix_certificate_not_after_timestamp_seconds"
	MetricCertificateRemainingSeconds         = "gonix_certificate_remaining_seconds"
	MetricCertificateDueForRenewal            = "gonix_certificate_due_for_renewal"
	MetricCertificateValid                    = "gonix_certificate_valid"
	MetricCertificateKeyMatches               = "gonix_certificate_key_matches"
	MetricCertificateChainValid               = "gonix_certificate_chain_valid"
	MetricCertificateStatus                   = "gonix_certificate_status"
	MetricCertificates                        = "gonix_certificates"
	MetricUpstreamUp                          = "gonix_upstream_up"
	MetricUpstreamConnectSeconds              = "gonix_upstream_connect_seconds"
	MetricNginxUp                             = "gonix_nginx_up"
	MetricNginxServiceActive                  = "gonix_nginx_service_active"
	MetricNginxServiceEnabled                 = "gonix_nginx_service_enabled"
	MetricNginxConfigValid                    = "gonix_nginx_config_valid"
	MetricNginxVersionInfo                    = "gonix_nginx_version_info"
	MetricNginxProcesses                      = "gonix_nginx_processes"
	MetricNginxUptimeSeconds                  = "gonix_nginx_uptime_seconds"
	MetricNginxMemoryResidentBytes            = "gonix_nginx_memory_resident_bytes"
	MetricNginxConnections                    = "gonix_nginx_connections"
	MetricNginxConnectionsAcceptedTotal       = "gonix_nginx_connections_accepted_total"
	MetricNginxConnectionsHandledTotal        = "gonix_nginx_connections_handled_total"
	MetricNginxHTTPRequestsTotal              = "gonix_nginx_http_requests_total"
	MetricNginxStubStatusUp                   = "gonix_nginx_stub_status_up"
	MetricHTTPRequestsTotal                   = "gonix_http_requests_total"
	MetricHTTPResponseBytesTotal              = "gonix_http_response_bytes_total"
	MetricErrorLogEntriesTotal                = "gonix_error_log_entries_total"
	MetricLogSizeBytes                        = "gonix_log_size_bytes"
	MetricLogLastWriteTimestampSeconds        = "gonix_log_last_write_timestamp_seconds"
	MetricLogDirFreeBytes                     = "gonix_log_dir_free_bytes"
	MetricLogDirTotalBytes                    = "gonix_log_dir_total_bytes"
	MetricExporterCollectorDurationSeconds    = "gonix_exporter_collector_duration_seconds"
	MetricExporterCollectorSuccess            = "gonix_exporter_collector_success"
	MetricBuildInfo                           = "gonix_build_info"
)

// Label names. LabelInstance is on every series.
const (
	LabelInstance  = "gonix_instance"
	LabelAction    = "action"
	LabelCode      = "code"
	LabelCollector = "collector"
	LabelDomain    = "domain"
	LabelGroup     = "group"
	LabelHost      = "host"
	LabelLevel     = "level"
	LabelManaged   = "managed"
	LabelMode      = "mode"
	LabelResult    = "result"
	LabelSeverity  = "severity"
	LabelSource    = "source"
	LabelSSL       = "ssl"
	LabelState     = "state"
	LabelStatus    = "status"
	LabelTarget    = "target"
	LabelType      = "type"
	LabelUpstream  = "upstream"
	LabelVersion   = "version"
)

// Type is a metric's type.
type Type string

const (
	Gauge   Type = "gauge"
	Counter Type = "counter"
)

// Def describes one metric.
type Def struct {
	Name   string
	Type   Type
	Help   string
	Labels []string // not counting LabelInstance
}

// Defs lists every metric the exporter can serve.
var Defs = []Def{
	{MetricHosts, Gauge, "Hosts by state; a host can be in several states.", []string{LabelState}},
	{MetricHostInfo, Gauge, "A host, with its mode (always 1).", []string{LabelHost, LabelMode, LabelManaged, LabelSSL}},
	{MetricHostEnabled, Gauge, "1 if the host is enabled.", []string{LabelHost}},
	{MetricHostMaintenance, Gauge, "1 if the host is in maintenance mode.", []string{LabelHost}},
	{MetricLintFindings, Gauge, "Config Check findings by severity.", []string{LabelSeverity}},
	{MetricLintTargetFindings, Gauge, "Config Check findings per host or shared file.", []string{LabelTarget, LabelSeverity}},
	{MetricAuditEventsTotal, Counter, "Audited changes since the exporter started.", []string{LabelAction, LabelResult}},
	{MetricLastChangeTimestampSeconds, Gauge, "Time of the last audited change, by result (failure = rolled back).", []string{LabelResult}},
	{MetricCertificateNotAfterTimestampSeconds, Gauge, "When the certificate expires.", []string{LabelDomain, LabelSource}},
	{MetricCertificateRemainingSeconds, Gauge, "Time left until expiry; negative once expired.", []string{LabelDomain, LabelSource}},
	{MetricCertificateDueForRenewal, Gauge, "1 if the certificate is within the renewal window.", []string{LabelDomain, LabelSource}},
	{MetricCertificateValid, Gauge, "1 if the certificate can be parsed and has not expired.", []string{LabelDomain, LabelSource}},
	{MetricCertificateKeyMatches, Gauge, "1 if the private key matches the certificate.", []string{LabelDomain, LabelSource}},
	{MetricCertificateChainValid, Gauge, "1 if the bundled chain verifies.", []string{LabelDomain, LabelSource}},
	{MetricCertificateStatus, Gauge, "The certificate's status (always 1).", []string{LabelDomain, LabelSource, LabelStatus}},
	{MetricCertificates, Gauge, "Certificates by status.", []string{LabelStatus}},
	{MetricUpstreamUp, Gauge, "1 if the backend accepts TCP connections.", []string{LabelHost, LabelGroup, LabelUpstream}},
	{MetricUpstreamConnectSeconds, Gauge, "Time to connect to the backend.", []string{LabelHost, LabelGroup, LabelUpstream}},
	{MetricNginxUp, Gauge, "1 if the Nginx master process is running.", nil},
	{MetricNginxServiceActive, Gauge, "1 if systemd reports the Nginx unit active.", nil},
	{MetricNginxServiceEnabled, Gauge, "1 if the Nginx unit starts at boot.", nil},
	{MetricNginxConfigValid, Gauge, "1 if nginx -t passes.", nil},
	{MetricNginxVersionInfo, Gauge, "Nginx version (always 1).", []string{LabelVersion}},
	{MetricNginxProcesses, Gauge, "Nginx processes by type.", []string{LabelType}},
	{MetricNginxUptimeSeconds, Gauge, "How long the master process has been running.", nil},
	{MetricNginxMemoryResidentBytes, Gauge, "Resident memory of the master and worker processes.", nil},
	{MetricNginxConnections, Gauge, "Current connections by state (stub_status).", []string{LabelState}},
	{MetricNginxConnectionsAcceptedTotal, Counter, "Accepted connections (stub_status).", nil},
	{MetricNginxConnectionsHandledTotal, Counter, "Handled connections (stub_status).", nil},
	{MetricNginxHTTPRequestsTotal, Counter, "Requests served by Nginx (stub_status).", nil},
	{MetricNginxStubStatusUp, Gauge, "1 if the stub_status page could be read.", nil},
	{MetricHTTPRequestsTotal, Counter, "Requests in the access log, by status code.", []string{LabelHost, LabelCode}},
	{MetricHTTPResponseBytesTotal, Counter, "Response body bytes sent, from the access log.", []string{LabelHost}},
	{MetricErrorLogEntriesTotal, Counter, "Entries in the error log, by level.", []string{LabelHost, LabelLevel}},
	{MetricLogSizeBytes, Gauge, "Size of a host's log file.", []string{LabelHost, LabelType}},
	{MetricLogLastWriteTimestampSeconds, Gauge, "When a host's access log was last written (no traffic if it stands still).", []string{LabelHost}},
	{MetricLogDirFreeBytes, Gauge, "Free space on the disk holding the Nginx logs.", nil},
	{MetricLogDirTotalBytes, Gauge, "Size of the disk holding the Nginx logs.", nil},
	{MetricExporterCollectorDurationSeconds, Gauge, "How long the collector's last run took.", []string{LabelCollector}},
	{MetricExporterCollectorSuccess, Gauge, "1 if the collector's last run succeeded.", []string{LabelCollector}},
	{MetricBuildInfo, Gauge, "GoNix version.", []string{LabelVersion}},
}

// Lookup returns the definition of the metric called name.
func Lookup(name string) (Def, bool) {
	for _, d := range Defs {
		if d.Name == name {
			return d, true
		}
	}
	return Def{}, false
}

// IsGonixMetric reports whether name looks like one of ours.
func IsGonixMetric(name string) bool { return strings.HasPrefix(name, "gonix_") }
