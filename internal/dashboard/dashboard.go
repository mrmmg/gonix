// Package dashboard generates GoNix's Grafana dashboard with Grafana's own
// Foundation SDK. Every query is written from the metric and label names
// defined in internal/metrics, and the tests check each one against those
// definitions, so the dashboard cannot drift from the exporter.
package dashboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/gauge"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	m "github.com/mrmmg/gonix/internal/metrics"
)

// UID identifies the dashboard, so importing it again updates it in place.
const UID = "gonix-overview"

var datasource = common.DataSourceRef{Type: ptr("prometheus"), Uid: ptr("${datasource}")}

func ptr[T any](v T) *T { return &v }

// sel renders a selector for metric. The instance filter is always there;
// the host filter is added when hostFilter is set and the metric has a host
// label. extra are further matchers such as `code=~"5.."`.
func sel(metric string, hostFilter bool, extra ...string) string {
	matchers := []string{m.LabelInstance + `=~"$instance"`}
	if def, ok := m.Lookup(metric); ok && hostFilter {
		for _, l := range def.Labels {
			if l == m.LabelHost {
				matchers = append(matchers, m.LabelHost+`=~"$host"`)
			}
		}
	}
	return metric + "{" + strings.Join(append(matchers, extra...), ",") + "}"
}

func perSecond(metric string, extra ...string) string {
	return fmt.Sprintf("rate(%s[$__rate_interval])", sel(metric, true, extra...))
}

// days shows a number of days as "12 days" (Grafana's own unit would turn it into weeks).
const days = "suffix: days"

const (
	red    = "red"
	orange = "orange"
	yellow = "yellow"
	green  = "green"
)

// steps builds absolute thresholds: the first color is the base, each
// following pair starts at its value.
func steps(base string, rest ...any) cog.Builder[dashboard.ThresholdsConfig] {
	s := []dashboard.Threshold{{Color: base}}
	for i := 0; i+1 < len(rest); i += 2 {
		s = append(s, dashboard.Threshold{Value: ptr(rest[i].(float64)), Color: rest[i+1].(string)})
	}
	return dashboard.NewThresholdsConfigBuilder().Mode(dashboard.ThresholdsModeAbsolute).Steps(s)
}

func query(expr, legend string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().Expr(expr).LegendFormat(legend).Datasource(datasource)
}

func last() cog.Builder[common.ReduceDataOptions] {
	return common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"})
}

type panel = cog.Builder[dashboard.Panel]

// number is a single-value tile.
func number(title, desc, unit, expr string, th cog.Builder[dashboard.ThresholdsConfig]) *stat.PanelBuilder {
	return stat.NewPanelBuilder().Datasource(datasource).Title(title).Description(desc).Unit(unit).Height(4).Span(4).
		Thresholds(th).ColorMode(common.BigValueColorModeValue).GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(last()).WithTarget(query(expr, ""))
}

type target struct{ expr, legend string }

func series(title, desc, unit string, targets ...target) *timeseries.PanelBuilder {
	p := timeseries.NewPanelBuilder().Datasource(datasource).Title(title).Description(desc).Unit(unit).Height(8).Span(12).
		FillOpacity(10).Legend(common.NewVizLegendOptionsBuilder().ShowLegend(true).DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom))
	for _, t := range targets {
		p.WithTarget(query(t.expr, t.legend))
	}
	return p
}

func bars(title, desc, unit string, th cog.Builder[dashboard.ThresholdsConfig], instant bool, expr, legend string) *bargauge.PanelBuilder {
	q := query(expr, legend)
	if instant {
		q.Instant()
	}
	return bargauge.NewPanelBuilder().Datasource(datasource).Title(title).Description(desc).Unit(unit).Height(8).Span(12).
		Thresholds(th).DisplayMode(common.BarGaugeDisplayModeBasic).Orientation(common.VizOrientationHorizontal).
		NamePlacement(common.BarGaugeNamePlacementLeft).ShowUnfilled(true).Min(0).
		ReduceOptions(last()).WithTarget(q)
}

// class matches the status codes of one class, e.g. class("5") is 500-599.
// stateline is a down/up timeline: red "Down" for 0, green "Up" for 1.
func stateline(title, desc string, height, span uint32) *statetimeline.PanelBuilder {
	state := func(text, color string, index int32) dashboard.ValueMappingResult {
		return dashboard.ValueMappingResult{Text: ptr(text), Color: ptr(color), Index: ptr(index)}
	}
	// The mappings name and color each state. The color mode must not be
	// "thresholds": a state timeline then ignores mappings and labels states
	// "< 1" and "1+".
	mappings := []dashboard.ValueMapping{{ValueMap: &dashboard.ValueMap{
		Type:    dashboard.MappingTypeValueToText,
		Options: map[string]dashboard.ValueMappingResult{"0": state("Down", red, 0), "1": state("Up", green, 1)},
	}}}
	return statetimeline.NewPanelBuilder().Datasource(datasource).Title(title).Description(desc).
		Mappings(mappings).Height(height).Span(span).
		ShowValue(common.VisibilityModeAuto).MergeValues(true)
}

func class(digit string) string { return fmt.Sprintf(`%s=~"%s.."`, m.LabelCode, digit) }

func requests(extra ...string) string { return perSecond(m.MetricHTTPRequestsTotal, extra...) }

// Build assembles the dashboard.
func Build() (dashboard.Dashboard, error) {
	b := dashboard.NewDashboardBuilder("GoNix").
		Uid(UID).
		Description("Nginx managed by GoNix: traffic, certificates, backends, the service and GoNix itself. Generated by `gonix exporter dashboard`.").
		Tags([]string{"gonix", "nginx"}).
		Refresh("30s").Time("now-6h", "now").Timezone(common.TimeZoneBrowser).Editable().
		WithVariable(dashboard.NewDatasourceVariableBuilder("datasource").Label("Data source").Type("prometheus")).
		WithVariable(dashboard.NewQueryVariableBuilder("instance").Label("Instance").Datasource(datasource).
			Query(dashboard.StringOrMap{String: ptr(fmt.Sprintf("label_values(%s, %s)", m.MetricBuildInfo, m.LabelInstance))}).
			Refresh(dashboard.VariableRefreshOnTimeRangeChanged).Sort(dashboard.VariableSortAlphabeticalAsc).
			Multi(true).IncludeAll(true).AllValue(".*")).
		WithVariable(dashboard.NewQueryVariableBuilder("host").Label("Host").Datasource(datasource).
			Query(dashboard.StringOrMap{String: ptr(fmt.Sprintf(`label_values(%s{%s=~"$instance"}, %s)`, m.MetricHostEnabled, m.LabelInstance, m.LabelHost))}).
			Refresh(dashboard.VariableRefreshOnTimeRangeChanged).Sort(dashboard.VariableSortAlphabeticalAsc).
			Multi(true).IncludeAll(true).AllValue(".*"))

	add := func(title string, panels ...panel) {
		b.WithRow(dashboard.NewRowBuilder(title))
		for _, p := range panels {
			b.WithPanel(p)
		}
	}

	add("Overview",
		number("Nginx running", "Whether the Nginx master process is running.", "bool_yes_no",
			"min("+sel(m.MetricNginxUp, false)+")", steps(red, 1.0, green)),
		number("Configuration valid", "Result of nginx -t.", "bool_yes_no",
			"min("+sel(m.MetricNginxConfigValid, false)+")", steps(red, 1.0, green)),
		number("Hosts enabled", "Enabled hosts, of all hosts GoNix knows.", "short",
			"sum("+sel(m.MetricHosts, false, m.LabelState+`="enabled"`)+")", steps(green)),
		number("Soonest certificate expiry", "Days left on the certificate that expires first.", days,
			fmt.Sprintf("min(%s) / 86400", sel(m.MetricCertificateRemainingSeconds, false)), steps(red, 14.0, orange, 30.0, green)).Decimals(0),
		number("Config Check errors", "Findings of severity error (gonix check).", "short",
			"sum("+sel(m.MetricLintFindings, false, m.LabelSeverity+`="error"`)+")", steps(green, 1.0, red)),
		number("5xx share of requests", "Server errors as a share of all requests.", "percentunit",
			fmt.Sprintf("(sum(%s) or vector(0)) / clamp_min(sum(%s) or vector(0), 1e-9)", requests(class("5")), requests()), steps(green, 0.01, yellow, 0.05, red)),
	)

	add("Traffic",
		series("Requests per second", "By status class, from the access logs.", "reqps",
			target{"sum(" + requests(class("2")) + ") or vector(0)", "2xx"},
			target{"sum(" + requests(class("3")) + ") or vector(0)", "3xx"},
			target{"sum(" + requests(class("4")) + ") or vector(0)", "4xx"},
			target{"sum(" + requests(class("5")) + ") or vector(0)", "5xx"}),
		series("5xx share by host", "Server errors as a share of each host's requests.", "percentunit",
			// A host without errors has no 5xx series; "or" keeps it on the graph at zero.
			target{fmt.Sprintf("(sum by (%[1]s) (%[2]s) / sum by (%[1]s) (%[3]s)) or (sum by (%[1]s) (%[3]s) * 0)", m.LabelHost, requests(class("5")), requests()), "{{host}}"}).Min(0),
		series("Requests by host", "", "reqps",
			target{fmt.Sprintf("sum by (%s) (%s)", m.LabelHost, requests()), "{{host}}"}),
		series("Bandwidth sent", "Response body bytes per second.", "Bps",
			target{fmt.Sprintf("sum by (%s) (%s)", m.LabelHost, perSecond(m.MetricHTTPResponseBytesTotal)), "{{host}}"}),
		series("Blocked and rate-limited", "403 and 429 answers: access lists, country blocking and rate limits at work.", "reqps",
			target{fmt.Sprintf("sum by (%s, %s) (%s)", m.LabelHost, m.LabelCode, requests(m.LabelCode+`=~"403|429"`)), "{{host}} {{code}}"}),
		series("Error log entries", "Entries in the error logs, by level.", "ops",
			target{fmt.Sprintf("sum by (%s) (%s)", m.LabelLevel, perSecond(m.MetricErrorLogEntriesTotal)), "{{level}}"}),
		bars("Busiest hosts", "Requests in the selected time range.", "short", steps(green), true,
			fmt.Sprintf("topk(10, sum by (%s) (increase(%s[$__range])))", m.LabelHost,
				sel(m.MetricHTTPRequestsTotal, true)), "{{host}}"),
	)

	day := 86400.0
	add("Certificates",
		number("Due for renewal", "Certificates within the renewal window.", "short",
			"sum("+sel(m.MetricCertificateDueForRenewal, false)+")", steps(green, 1.0, yellow)).Span(6),
		number("Expired or invalid", "Certificates that are expired or cannot be read.", "short",
			"sum("+sel(m.MetricCertificates, false, m.LabelStatus+`=~"expired|invalid"`)+")", steps(green, 1.0, red)).Span(6),
		number("Broken chains", "Certificates whose bundled chain does not verify.", "short",
			"count("+sel(m.MetricCertificateChainValid, false)+" == 0) or vector(0)", steps(green, 1.0, red)).Span(6),
		number("Key mismatches", "Certificates whose private key does not match.", "short",
			"count("+sel(m.MetricCertificateKeyMatches, false)+" == 0) or vector(0)", steps(green, 1.0, red)).Span(6),
		bars("Days until expiry", "The 15 certificates that expire first; negative once expired.", days, steps(red, 7.0, orange, 14.0, yellow, 30.0, green), true,
			fmt.Sprintf("bottomk(15, %s / %v)", sel(m.MetricCertificateRemainingSeconds, false), day), "{{domain}}").Span(24).Height(14).Decimals(0),
	)

	add("Upstreams",
		stateline("Backend reachable", "Whether each backend accepts TCP connections; groups included.", 8, 12).
			WithTarget(query(sel(m.MetricUpstreamUp, true), "{{host}} → {{upstream}}")),
		series("Backend connect time", "Time to open a TCP connection to each backend.", "s",
			target{sel(m.MetricUpstreamConnectSeconds, true), "{{host}} → {{upstream}}"}),
	)

	add("Nginx",
		series("Connections", "Needs stub_status (gonix exporter set --stub-status on).", "short",
			target{fmt.Sprintf("sum by (%s) (%s)", m.LabelState, sel(m.MetricNginxConnections, false)), "{{state}}"}),
		series("Requests served by Nginx", "As Nginx counts them (stub_status).", "reqps",
			target{"sum(rate(" + sel(m.MetricNginxHTTPRequestsTotal, false) + "[$__rate_interval]))", "requests"},
			target{"sum(rate(" + sel(m.MetricNginxConnectionsAcceptedTotal, false) + "[$__rate_interval]))", "accepted connections"}),
		series("Processes", "", "short",
			target{fmt.Sprintf("sum by (%s) (%s)", m.LabelType, sel(m.MetricNginxProcesses, false)), "{{type}}"}).Span(8),
		series("Memory", "Resident memory of the master and workers.", "bytes",
			target{"sum(" + sel(m.MetricNginxMemoryResidentBytes, false) + ")", "resident"}).Span(8),
		number("Uptime", "Of the Nginx master process.", "s",
			"min("+sel(m.MetricNginxUptimeSeconds, false)+")", steps(green)).Span(8),
	)

	add("Logs and disk",
		gauge.NewPanelBuilder().Datasource(datasource).Title("Free space for logs").Description("On the disk that holds the Nginx logs.").
			Unit("percentunit").Min(0).Max(1).Height(8).Span(6).Thresholds(steps(red, 0.1, orange, 0.2, green)).
			ReduceOptions(last()).
			WithTarget(query(fmt.Sprintf("min(%s / %s)", sel(m.MetricLogDirFreeBytes, false), sel(m.MetricLogDirTotalBytes, false)), "")),
		bars("Access log size", "Per host.", "bytes", steps(green), true,
			fmt.Sprintf("topk(10, %s)", sel(m.MetricLogSizeBytes, true, m.LabelType+`="access"`)), "{{host}}").Span(9),
		bars("Time since the last request was logged", "A host with no traffic stands still.", "s", steps(green), true,
			fmt.Sprintf("time() - %s", sel(m.MetricLogLastWriteTimestampSeconds, true)), "{{host}}").Span(9),
	)

	add("GoNix",
		series("Changes", "Audited changes; a failure was rolled back.", "short",
			target{fmt.Sprintf("sum by (%s) (increase(%s[$__rate_interval]))", m.LabelResult, sel(m.MetricAuditEventsTotal, false)), "{{result}}"}).Span(8),
		series("Hosts by state", "", "short",
			target{fmt.Sprintf("sum by (%s) (%s)", m.LabelState, sel(m.MetricHosts, false)), "{{state}}"}).Span(8),
		series("Config Check findings", "", "short",
			target{fmt.Sprintf("sum by (%s) (%s)", m.LabelSeverity, sel(m.MetricLintFindings, false)), "{{severity}}"}).Span(8),
		number("Rolled back in range", "Changes that failed and were rolled back in the selected range.", "short",
			fmt.Sprintf("sum(increase(%s[$__range]))", sel(m.MetricAuditEventsTotal, false, m.LabelResult+`="failure"`)), steps(green, 1.0, yellow)).Height(6),
		number("Last change", "", "dateTimeFromNow",
			"max("+sel(m.MetricLastChangeTimestampSeconds, false, m.LabelResult+`="success"`)+") * 1000", steps(green)).Height(6),
		stateline("Exporter collectors", "Up while a collector works; Down when its last run failed.", 6, 16).
			WithTarget(query(fmt.Sprintf("min by (%s) (%s)", m.LabelCollector, sel(m.MetricExporterCollectorSuccess, false)), "{{collector}}")),
	)

	return b.Build()
}

// JSON renders the dashboard as the file Grafana imports.
func JSON() ([]byte, error) {
	d, err := Build()
	if err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// FileName is the name used when a directory is given instead of a file.
const FileName = "gonix-dashboard.json"

// Save writes the dashboard to path, replacing an older version, and returns
// where it went. If path is a directory, the file is FileName inside it.
func Save(path string) (string, error) {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		path = filepath.Join(path, FileName)
	}
	data, err := JSON()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	// Write beside the target and rename, so a reader (Grafana's provisioning
	// watches the folder) never sees a half-written file.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gonix-dashboard-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}
