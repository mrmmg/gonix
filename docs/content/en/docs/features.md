---
title: Features
weight: 5
---

> **What can GoNix actually do for me?**
> Everything below, from the menu or from the command line. Each item links to the page that explains it.

## Hosts and routing

- **Host management.** Create, list, enable, disable and delete virtual hosts. They are named after their domain
  (`sites-available/example.com`, not an opaque ID) and linked into `sites-enabled` the standard Nginx way. See [Hosts]({{< relref "hosts" >}}).
- **Reverse proxy and static hosts.** Guided setup for upstream host and port, protocol, HTTP version, timeouts, body size limits and forwarded headers.
- **Locations.** Add more reverse-proxy locations to a host, or a fully custom location block for advanced cases.
- **WebSocket support.** One Yes/No toggle sets the right `Upgrade`, `Connection` and `proxy_http_version` directives.
- **Redirect hosts and domain aliases.** Redirect a whole domain (`example.net` to `https://example.com`) with 301, 302, 307 or 308, keeping or dropping the path, and serve several names (`www.example.com`) from one host.
- **HTTP/2 and HTTP/3.** The right `http2` syntax for your Nginx version; HTTP/3 (QUIC) is offered only when your build supports it.
- **Coexists with hand-written configuration.** Hosts GoNix did not create are shown read-only with an `[unmanaged]` marker instead of being rewritten. See [How it works]({{< relref "how-it-works" >}}).
- **Adopts hand-written hosts and certbot setups.** `gonix adopt` manages an existing file without rewriting it: only the directives a change involves are edited, everything else stays as written, and certbot keeps renewing its certificates. See [Migration]({{< relref "migration#adopting-hand-written-hosts-and-certbot" >}}).

## Certificates

- **Centralized SSL/TLS.** Hosts reference `/etc/nginx/certs/<domain>/{fullchain,privkey}.pem`, so renewing is replacing two files. RSA and ECDSA are detected automatically. See [SSL certificates]({{< relref "ssl" >}}).
- **Certificate inspection.** Expiry (human-readable and exact), issuer, subject, SANs, SHA-256 fingerprint, key/certificate match and chain verification, using Go's `crypto/x509` with no `openssl` subprocess.
- **GoNix's and Let's Encrypt's certificates, side by side.** The Certificates menu, the Add Host wizard and SSL Configuration list GoNix's own certificates and certbot's (`certificates.letsencrypt_dir`, default `/etc/letsencrypt`), each tagged `[GoNix]` or `[Let's Encrypt]`; a host's own custom certificate is kept and tagged `[Custom]`.
- **Expiry warnings and certbot renewal.** A certificate with 14 days or less left is shown in red in the list, on the host's SSL line and on the main menu. When `certbot` is installed, a Let's Encrypt certificate can be renewed from its menu.
- **Wildcard certificates.** Issue and renew `*.domain.com` with `certbot` and a Cloudflare DNS-01 challenge, fully automated from the TUI.

## Access control

- **Access lists (HTTP Basic Auth).** Reusable, bcrypt-hashed `htpasswd` user lists for a host or a location. See [Access control]({{< relref "access-control" >}}).
- **IP access lists.** Reusable allow or block lists of IPs and CIDR ranges, combinable with Basic Auth ("IP *or* password" with `satisfy any`).
- **Country blocking.** Allow or block countries per host with the geoip2 module and a MaxMind database.
- **Real client IP.** Trust Cloudflare (bundled or freshly downloaded ranges) or your own proxies, so logs, IP lists and rate limits see the visitor.

## Traffic and resilience

- **Host presets.** One-click security headers, HSTS, gzip, static asset caching, proxy caching, sensitive file blocking and bad bot blocking. See [Traffic shaping]({{< relref "traffic" >}}).
- **Rate limiting.** Named request and connection limits (with `general`, `api` and `login` presets), per host or location.
- **Maintenance mode.** Serve a 503 page while your own IPs still get through.
- **Error pages.** Reusable, named snippets that map HTTP errors (502, 503, 504...) to your own HTML. See [Error pages]({{< relref "error-pages" >}}).
- **Upstream groups.** Load balancing (round robin, least connections, IP hash) with weights, backup servers and passive health checks. See [Load balancing and streams]({{< relref "load-balancing" >}}).
- **Stream (TCP/UDP) proxies.** Forward databases, game servers or any TCP/UDP port, optionally behind an IP list.

## Visibility

- **Access and error logs.** Per host, with logrotate configured on install.
- **Live logs.** Follow requests with filters (`status:5xx`, `ip:`, `path:`), a summary of top IPs, paths and statuses, and block an abusive IP straight from the log view. See [Logs and monitoring]({{< relref "logs-and-monitoring" >}}).
- **Audit log.** Every change is recorded with timestamp, Linux user, action, target and result.
- **Config Check.** Finds duplicate server names, missing files, expiring or mismatched certificates, unreachable upstreams, unused snippets and other common mistakes.
- **Prometheus and Grafana.** `gonix exporter` serves metrics for certificates, backends, hosts, Nginx and traffic, runs as a systemd service GoNix controls itself, and `gonix exporter dashboard` writes a matching Grafana dashboard. See [Prometheus and Grafana]({{< relref "prometheus-grafana" >}}).
- **Service control and monitoring.** Start, stop, restart, reload and test Nginx, with master and worker discovery, memory, uptime and listening ports read from `/proc`.

## Safety and migration

- **Updates.** The TUI says when a newer GoNix is released (a daily check, which can be switched off in Settings) and `gonix update` installs it from GitHub Releases: SHA-256 verified, the previous version kept, rollback with `--rollback`.
- **Safety first.** Every change is backed up, written, tested with `nginx -t`, and reloaded only if the test passes. A failed test restores the previous configuration and logs the failure. Nginx is never left broken. See [Backups and history]({{< relref "backups-history" >}}).
- **Configuration history.** Every successful change is a Git commit, kept outside `/etc/nginx`. Browse diffs and restore any earlier state.
- **Export and import.** Move everything GoNix manages to another server in one archive. See [Migration]({{< relref "migration" >}}).
- **Nginx Proxy Manager import.** Bring proxy hosts, redirects, streams, certificates and access lists from an existing NPM.

## Automation

- **Command line.** Every action is also a scriptable command with `--json` and `--dry-run`. See [Command line]({{< relref "cli" >}}).
- **AI agents.** A bundled agent skill and an MCP server let tools like Claude Code manage Nginx safely. See [AI agents]({{< relref "ai-agents" >}}).
- **Multiple Nginx instances.** Manage several Nginx installations from one GoNix with `--instance`. See [Multiple instances]({{< relref "instances" >}}).
