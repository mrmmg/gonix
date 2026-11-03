---
title: Load balancing and streams
weight: 90
---

> **One backend is a single point of failure. And not everything you proxy speaks HTTP.**
> Upstream groups spread HTTP load, and stream proxies forward raw TCP/UDP.

## Upstream groups

An **upstream group** is a named set of backends. Hosts send traffic to the group instead of one URL.

```bash
sudo gonix upstream create app \
  --servers "10.0.0.11:3000 weight=3, 10.0.0.12:3000, 10.0.0.13:3000 backup" \
  --method least_conn
sudo gonix host add example.com --group app
```

| `--method` | Behaviour | Pick it when |
|---|---|---|
| `round_robin` (default) | Take turns | Requests cost about the same |
| `least_conn` | Fewest active connections | Requests vary a lot in duration |
| `ip_hash` | Same client, same backend | You need simple stickiness |

- **Weights** (`weight=3`) send proportionally more traffic to stronger machines.
- **Backup servers** (`backup`) receive traffic only when the others are down.
- **Passive health checks**: a failing server is taken out of rotation for a while. This is all open-source
  Nginx offers; active checks are on the [roadmap]({{< relref "roadmap" >}}).

`gonix check` also tests that your upstreams are reachable.

## Stream (TCP/UDP) proxies

> **How do you put a database, a game server or an SSH jump host behind Nginx?**
> With the `stream` module, which is plain TCP/UDP forwarding, not HTTP.

```bash
sudo gonix stream setup
sudo gonix stream create pg --port 5432 --backend 10.0.0.20:5432 --ip-list office
sudo gonix stream create dns --port 53 --backend 10.0.0.2:53 --udp
sudo gonix stream list
```

- `stream setup` creates the `stream { }` block when `nginx.conf` has none, using `nginx.main_include_dir`.
- `--ip-list` limits who may connect, using the same [IP lists]({{< relref "access-control" >}}).
- Stream files go to `nginx.streams_dir`.

This needs the stream module (Debian/Ubuntu: `libnginx-mod-stream`).

{{< callout type="warning" >}}
**Exposing a database port to the internet is a decision, not a default.** Put an IP list on every stream
that carries something sensitive.
{{< /callout >}}
