---
title: Prometheus and Grafana
weight: 105
---

> **Can I see my certificates, backends and error rates on a Grafana dashboard?**
> Yes. `gonix exporter` serves everything GoNix knows as Prometheus metrics, and GoNix runs and configures
> the exporter itself.

The exporter is a small service (`gonix exporter run`) that answers `GET /metrics`. GoNix owns its systemd
unit and its settings, so changing the port or a filter is one command (or one screen in the TUI) and never
means editing a unit file by hand.

## Turn it on

```bash
sudo gonix exporter enable      # installs the systemd unit; starts now and at every boot
sudo gonix exporter status
curl -s http://127.0.0.1:9113/metrics | head
```

In the TUI: **Monitoring** in the main menu. It shows whether the exporter runs, lets you enable, disable,
start, stop and restart it, change its settings, preview a scrape and read the systemd unit.

| Command | Does |
|---|---|
| `exporter enable` / `disable` | Install the unit and start it now and at boot / stop it and keep it from starting |
| `exporter start` / `stop` / `restart` | Control it without changing boot behavior |
| `exporter uninstall` | Remove the unit (settings are kept) |
| `exporter set ...` | Change settings, then restart it if it runs |
| `exporter status` | Service state and settings (`--json` too) |
| `exporter metrics` | Print one scrape, to check without Prometheus |

## Settings

Settings live in the `monitoring:` section of `gonix.yaml`, next to GoNix's other settings. Use
`gonix exporter set` (or the TUI) rather than editing the file: it validates, saves, applies and restarts.

```bash
sudo gonix exporter set --listen 127.0.0.1:9200          # new port; the unit does not change
sudo gonix exporter set --hosts '*.example.com' --exclude-hosts 'staging.*'
sudo gonix exporter set --collectors certificates,upstreams,nginx
sudo gonix exporter set --cache-ttl 1m
sudo gonix exporter set --stub-status on                 # connection counters
```

```yaml
monitoring:
  listen: 127.0.0.1:9113      # loopback unless you change it on purpose
  hosts: []                   # only these hosts (patterns); empty = all
  exclude_hosts: []
  collectors: []              # empty = all of them
  cache_ttl: 30s              # certificates, backends and nginx -t are reused this long
  stub_status:
    enabled: false
    listen: 127.0.0.1:9114
```

The unit never contains a setting; it runs `gonix exporter run --config <file>`. That is why a new port needs
no unit edit. If GoNix itself moves, `exporter restart` rewrites the unit.

### Filtering per scrape

Prometheus can narrow a job without changing GoNix:

```text
/metrics?host=*.example.com&host=api.example.org   only these hosts
/metrics?exclude_host=staging.*                    leave these out
/metrics?collect[]=certificates&collect[]=nginx    only these collectors
```

```yaml
scrape_configs:
  - job_name: gonix-certificates
    params: { 'collect[]': [certificates] }
    static_configs: [{ targets: ['127.0.0.1:9113'] }]
```

Series that are not about one host (the Nginx service, disk space) are never filtered out.

## Metrics

Every series carries a `gonix_instance` label. Collectors can be switched on and off.

| Collector | Metrics |
|---|---|
| `gonix` | `gonix_hosts{state}`, `gonix_host_info`, `gonix_host_enabled`, `gonix_host_maintenance`, `gonix_lint_findings{severity}`, `gonix_lint_target_findings`, `gonix_audit_events_total{action,result}`, `gonix_last_change_timestamp_seconds{result}` |
| `certificates` | `gonix_certificate_remaining_seconds`, `_not_after_timestamp_seconds`, `_due_for_renewal`, `_valid`, `_key_matches`, `_chain_valid`, `_status`, `gonix_certificates{status}` (labels `domain`, `source`) |
| `upstreams` | `gonix_upstream_up`, `gonix_upstream_connect_seconds` (labels `host`, `group`, `upstream`; upstream groups included) |
| `nginx` | `gonix_nginx_up`, `_service_active`, `_service_enabled`, `_config_valid` (`nginx -t`), `_version_info`, `_processes{type}`, `_uptime_seconds`, `_memory_resident_bytes`; with stub_status: `gonix_nginx_connections{state}`, `_connections_accepted_total`, `_connections_handled_total`, `gonix_nginx_http_requests_total` |
| `traffic` | `gonix_http_requests_total{host,code}`, `gonix_http_response_bytes_total{host}`, `gonix_error_log_entries_total{host,level}` |
| `logfiles` | `gonix_log_size_bytes{host,type}`, `gonix_log_last_write_timestamp_seconds{host}`, `gonix_log_dir_free_bytes`, `gonix_log_dir_total_bytes` |
| (always) | `gonix_build_info`, `gonix_exporter_collector_duration_seconds`, `gonix_exporter_collector_success` |

Blocked requests need no metric of their own: they are `code="403"` and `code="429"`.

Things to know:

- **Traffic counters start at zero** when the exporter starts and count what is appended to the access
  logs from then on, following log rotation. Use `rate()` and `increase()` as with any counter; a restart
  looks like any other counter reset. A preview (`exporter metrics`) therefore shows no traffic.
- Latency is not exported: the default log format has no request time.
- `stub_status` needs Nginx's `http_stub_status_module`. GoNix adds a server block that listens on the
  configured address and answers local clients only; if Nginx refuses it, `nginx -t` rolls it back.
- Hosts whose access log GoNix does not name `<domain>.access.log` (some adopted hosts) are not counted.

## Grafana dashboard

GoNix writes a ready-made dashboard for exactly these metrics:

```bash
sudo gonix exporter dashboard --out /var/lib/grafana/dashboards/    # file, or a folder (gonix-dashboard.json)
gonix exporter dashboard > gonix-dashboard.json                      # or stdout; needs no root
```

In the TUI: **Monitoring**, then **Save Grafana Dashboard**, which asks for the path. Run it again after
upgrading GoNix: an older copy is replaced, and because the dashboard is generated by the same GoNix that
exports the metrics, the two always match. There is no version to look up.

Import it in Grafana (Dashboards, New, Import), or put it in a folder watched by
[provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/#dashboards). It asks
for a Prometheus data source and has three variables: **Instance** (every GoNix instance scraped),
**Host** and the data source. Its seven rows are:

| Row | Shows |
|---|---|
| Overview | Nginx running, `nginx -t`, hosts, soonest certificate expiry, Config Check errors, 5xx share |
| Traffic | Requests per second by status class, 5xx share per host, bandwidth, 403/429 blocks, error log entries, busiest hosts |
| Certificates | Days until expiry per certificate, due for renewal, expired, broken chains, key mismatches |
| Upstreams | Whether each backend is reachable, and its connect time |
| Nginx | Connections and requests (with stub_status), processes, memory, uptime |
| Logs and disk | Free space for logs, log sizes, time since a host's last request |
| GoNix | Audited changes and rollbacks, hosts by state, findings, collector health |

The dashboard has no alert rules. Panels whose collector you switched off stay empty; "Connections" needs
`stub_status`.

The generator is built from the same definitions the exporter uses (`internal/metrics/defs.go`), and tests
check every query against them: that the metric and its labels exist, that counters are only used with
`rate()` or `increase()`, and that the variables are defined.

## Multiple instances

Each [instance]({{< relref "instances" >}}) has its own exporter, unit and settings:

```bash
sudo gonix --instance staging exporter enable      # unit: gonix-exporter-staging
```

An instance that does not set its own addresses gets a stable port of its own (derived from its name), so
two exporters never fight over `9113`. `exporter enable` and `start` refuse an address that is already in
use and say how to pick another.
