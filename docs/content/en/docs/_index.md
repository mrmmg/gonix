---
title: Documentation
weight: 1
---

> **Have you ever been afraid to run `systemctl reload nginx`?**
> That moment of doubt (did I leave a stray semicolon, is the certificate path right, will this
> take down every site on the server?) is exactly what GoNix exists to remove.

These docs are written to be read, not just searched. Each page starts with a question you probably
already have, shows the trouble you run into, and then shows how GoNix solves it, with a reason for
every design choice, so you remember *why*, not only *what*.

## What is GoNix?

The name comes from **Go** + **Nginx**: an advanced, interactive terminal UI (TUI) for managing Nginx on Linux, written in Go.
The compiled binary is called `gonix`.

It is inspired by [Nginx Proxy Manager](https://nginxproxymanager.com/) but takes a different road. Instead of hiding Nginx
behind a database and a proprietary format, GoNix is a thin, safe layer **on top of** your real, human-editable Nginx files. You can always open
`/etc/nginx/sites-available/example.com` in a text editor and see plain, standard Nginx syntax: nothing opaque, nothing proprietary.

{{< preview >}}

Curious about everything it can do? See [Features]({{< relref "features" >}}).

## Where to go

{{< cards >}}
  {{< card link="features" title="Features" subtitle="Everything GoNix can do, in one list." icon="sparkles" >}}
  {{< card link="install" title="Install" subtitle="One command, and what ends up where." icon="download" >}}
  {{< card link="quickstart" title="Quickstart" subtitle="Your first HTTPS site in five minutes." icon="play" >}}
  {{< card link="how-it-works" title="How it works" subtitle="The three ideas behind GoNix. Read this once." icon="light-bulb" >}}
  {{< card link="hosts" title="Hosts" subtitle="Proxies, static sites, redirects, WebSockets." icon="server" >}}
  {{< card link="ssl" title="SSL certificates" subtitle="Layout, inspection and wildcard certificates." icon="lock-closed" >}}
  {{< card link="access-control" title="Access control" subtitle="Passwords, IP lists, countries, real client IP." icon="shield-check" >}}
  {{< card link="traffic" title="Traffic shaping" subtitle="Presets, rate limits, maintenance mode." icon="adjustments" >}}
  {{< card link="load-balancing" title="Load balancing and streams" subtitle="Upstream groups and TCP/UDP proxies." icon="switch-horizontal" >}}
  {{< card link="logs-and-monitoring" title="Logs and monitoring" subtitle="Live logs, audit log, Config Check." icon="chart-bar" >}}
  {{< card link="backups-history" title="Backups and history" subtitle="Undo anything." icon="clock" >}}
  {{< card link="migration" title="Migration" subtitle="Move servers, or leave Nginx Proxy Manager." icon="truck" >}}
  {{< card link="cli" title="Command line" subtitle="Scripts, CI and the full command reference." icon="terminal" >}}
  {{< card link="ai-agents" title="AI agents" subtitle="A skill and an MCP server for Claude Code and friends." icon="chip" >}}
  {{< card link="instances" title="Multiple instances" subtitle="Several Nginx installations, one GoNix." icon="collection" >}}
  {{< card link="configuration" title="Configuration reference" subtitle="Every key of gonix.yaml." icon="cog" >}}
  {{< card link="troubleshooting" title="Troubleshooting" subtitle="Symptoms, causes and fixes." icon="exclamation" >}}
  {{< card link="contributing" title="Contributing" subtitle="Build from source, run tests, project layout." icon="code" >}}
  {{< card link="roadmap" title="Roadmap" subtitle="What is deliberately not here yet." icon="map" >}}
{{< /cards >}}

## How to read the examples

- Lines starting with `$` are what you type. Almost everything needs root, so commands are shown
  with `sudo`.
- Blocks marked `nginx` show the configuration GoNix writes for you, so you can see there is no magic.
- Every change command accepts `--dry-run`. When you are unsure, add it and read the diff first.
