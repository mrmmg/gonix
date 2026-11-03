---
title: Logs and monitoring
weight: 100
---

> **A user says "the site is slow". Where do you look first?**
> Not in a 2 GB log file with `grep`. GoNix keeps three kinds of evidence, each answering a different question.

| Log | Answers | Where |
|---|---|---|
| Access / error logs | What is hitting this host? | `/var/log/nginx/<domain>.access.log`, `<domain>.error.log` |
| Audit log | Who changed what, and did it work? | `/var/log/gonix/audit.log` |
| Config Check | Is anything misconfigured right now? | on demand |

## Access and error logs

Per host, toggled independently from the host's screen (CLI: `--access-log on|off`, `--error-log on|off`).
`logrotate` is configured by the installer in `/etc/logrotate.d/gonix`.

### Live logs

```bash
sudo gonix logs app.example.com --lines 50
sudo gonix logs app.example.com --filter "status:5xx" --summary
sudo gonix logs app.example.com --error
```

Filters combine: `status:5xx`, `ip:203.0.113.9`, `path:/login`. `--summary` shows the top IPs, paths and
statuses, which is usually the answer to "who is hammering me?". In the TUI, the live view follows requests
as they arrive and lets you **block an abusive IP straight from the log**, which adds it to a block list.

{{< callout type="info" >}}
Behind a proxy, enable [real client IP]({{< relref "access-control" >}}) first, or every log line shows the proxy.
{{< /callout >}}

## Audit log

One line per action:

```text
2026-09-15 10:32:11 | user=root | action=host_created | target=example.com | result=success
```

Changes from the CLI add `| actor=cli` (or the value of `GONIX_ACTOR`); the MCP server uses `actor=mcp`.
Each extra [instance]({{< relref "instances" >}}) has its own `/var/log/gonix/<instance>-audit.log`.

```bash
sudo gonix audit --lines 20
```

Failures are recorded with Nginx's error output, so "why was this rolled back?" always has an answer.

## Config Check

> **Is it the certificate, a duplicate server name, or a dead backend?**
> Config Check looks for all of these at once.

```bash
sudo gonix check
sudo gonix check --no-upstreams --json
```

It finds duplicate server names, missing files, expiring or mismatched certificates, unreachable upstreams,
unused snippets, and a missing include of `nginx.http_snippets_dir`. Use `--no-upstreams` to skip the
network checks.

## Nginx service and monitoring

Start, stop, restart, reload and test Nginx from the TUI, with master/worker discovery, memory, uptime and
listening ports read directly from `/proc`. From the CLI:

```bash
sudo gonix nginx status
sudo gonix nginx test
sudo gonix nginx reload
```
