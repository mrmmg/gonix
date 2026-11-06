---
title: Command reference
weight: 1
---

`gonix help` is always the authoritative, current list. This page groups the same commands by task.
Commands marked **✎** change the configuration and accept `--dry-run`.

## Hosts

| Command | Description |
|---|---|
| `host list` | List hosts |
| `host show <domain> [--raw]` | Show settings, or the raw configuration |
| ✎ `host add <domain> (--upstream URL \| --group NAME \| --root DIR \| --redirect URL) [flags]` | Create and enable a host |
| ✎ `host set <domain> [flags]` | Change a host's settings |
| ✎ `host enable <domain>` / `host disable <domain>` | Enable, or disable while keeping the file |
| ✎ `host delete <domain> --yes` | Delete a host (a backup is kept) |
| ✎ `host path add <domain> <path> (--upstream URL \| --group NAME) [--websocket]` | Add a reverse-proxy location |
| ✎ `host path remove <domain> <path>` | Remove a location |

Common `host add` / `host set` flags:

| Flag | Meaning |
|---|---|
| `--ssl NAME` | Certificate directory name (`none` to detach) |
| `--http2`, `--http3`, `--force-https` | HTTP versions and the HTTP-to-HTTPS redirect |
| `--presets a,b` | [Presets]({{< relref "../traffic" >}}) |
| `--ip-list`, `--country-list`, `--access-list`, `--rate-limit`, `--error-pages` | Attach a named policy (`none` to detach) |
| `--satisfy any\|all` | IP list *or* / *and* Basic Auth |
| `--alias NAME` (add) / `--aliases "a b"` (set) | Additional domain names |
| `--websocket on\|off` | WebSocket support |
| `--maintenance on\|off`, `--bypass "IP ..."` | Maintenance mode |
| `--access-log`, `--error-log` | `on` or `off` |
| `--connect-timeout`, `--read-timeout`, `--send-timeout`, `--max-body` | Proxy tuning |
| `--code`, `--no-preserve-path` | Redirect status and path handling |
| `--no-forward-headers` | Do not send `Host`/`X-Forwarded-*` |
| `--disabled` | Create the host without enabling it |

## Access control

| Command | Description |
|---|---|
| `accesslist list` | List Basic Auth access lists |
| ✎ `iplist create <name> --mode allow\|block --entries "IP/CIDR ..."` | Create an IP list |
| ✎ `iplist add <name> <ip/cidr>` / `iplist remove <name> <ip/cidr>` | Edit a list |
| ✎ `iplist delete <name> --yes` | Delete an unused list |
| `iplist list` | List IP lists |
| ✎ `country setup` | Load the GeoIP database |
| ✎ `country create <name> --mode allow\|block --countries "DE FR ..."` | Create a country list |
| ✎ `country delete <name> --yes` | Delete an unused list |
| `country list` | List country lists |
| `realip show` | Show real client IP settings |
| ✎ `realip cloudflare [--refresh]` | Trust Cloudflare |
| ✎ `realip custom --proxies "IP/CIDR ..." --header X-Forwarded-For\|X-Real-IP` | Trust your own proxies |
| ✎ `realip disable` | Stop trusting proxy headers |

## Traffic, balancing and streams

| Command | Description |
|---|---|
| `preset list` | List host presets |
| ✎ `ratelimit create <name> [--preset general\|api\|login] [--rate N --per s\|m --burst N --nodelay --conn N]` | Create a rate limit |
| ✎ `ratelimit delete <name> --yes` / `ratelimit list` | Delete / list |
| ✎ `upstream create <name> --servers "host:port[ weight=N][ backup], ..." [--method round_robin\|least_conn\|ip_hash]` | Create an upstream group |
| ✎ `upstream delete <name> --yes` / `upstream list` | Delete / list |
| ✎ `stream setup` | Create the `stream { }` block |
| ✎ `stream create <name> --port N --backend host:port [--udp] [--ip-list NAME]` | Create a stream proxy |
| ✎ `stream enable\|disable\|delete <name>` / `stream list` | Manage / list |
| `errorpages list` | List error pages snippets |

## Observe

| Command | Description |
|---|---|
| `logs <domain> [--lines N] [--filter "status:5xx ip:... path:..."] [--summary] [--error]` | Recent requests or errors |
| `audit [--lines N]` | Latest audited changes |
| `check [--no-upstreams]` | Config Check |
| `cert list [--letsencrypt DIR\|none]` | Certificates and their expiry, certbot's included |
| `nginx status\|test\|reload` | Service control |
| `exporter enable\|disable\|start\|stop\|restart\|uninstall` | Run the Prometheus exporter as a systemd service |
| `exporter status` | Service state and settings |
| `exporter set [--listen ADDR] [--hosts P,..\|none] [--exclude-hosts P,..\|none] [--collectors C,..\|all] [--cache-ttl 30s] [--stub-status on\|off]` | Change its settings; see [Prometheus and Grafana]({{< relref "prometheus-grafana" >}}) |
| `exporter metrics [--host P]... [--exclude-host P]... [--collect NAME]...` | Print one scrape |
| `exporter dashboard [--out FILE\|DIR]` | Write the Grafana dashboard for these metrics (stdout if no `--out`; needs no root) |
| `exporter run` | Serve `/metrics` in the foreground (what the unit runs) |

## History and moving

| Command | Description |
|---|---|
| `history list [--lines N]` / `history show <commit>` | Browse history |
| `history restore <commit> --yes` | Restore a commit |
| `export <file.tar.gz \| ->` | Export everything GoNix manages |
| ✎ `import <file.tar.gz> [--overwrite --yes]` | Import an export |
| ✎ `npm-import --data DIR [--letsencrypt DIR] [--disabled]` | Import from Nginx Proxy Manager |
| ✎ `adopt (<domain\|file>... \| --all) [--verify-renewal]` | Manage hand-written host files (only a marker comment is added) |
| ✎ `adopt release <domain>` | Stop managing an adopted host file (its content is kept) |

## Tooling

| Command | Description |
|---|---|
| `instances` | List the configured Nginx instances |
| `skill install [--project \| --dir DIR]` / `skill print` | [AI agent skill]({{< relref "../ai-agents" >}}) |
| `mcp` | MCP server on stdio |
| `--version`, `-v` | Print the version |
