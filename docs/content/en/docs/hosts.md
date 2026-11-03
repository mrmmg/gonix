---
title: Hosts
weight: 40
---

> **A host is "just" a `server {}` block. So why not write it by hand?**
> You can, and GoNix will happily show you the result. But the 40 lines that make a *correct* reverse
> proxy (forwarded headers, WebSocket upgrade, timeouts, HTTPS redirect) are the same every time, and
> a typo in any of them takes the site down. GoNix writes them once, correctly.

## Reverse proxy

The most common host: send a domain to an application.

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000
```

In the TUI, **Add New Host** walks you through the same options: upstream host and port, protocol,
HTTP version, timeouts, body size limit and forwarded headers.

| Option | Flag | Why you might change it |
|---|---|---|
| Connect / read / send timeout | `--connect-timeout`, `--read-timeout`, `--send-timeout` | Slow reports, long polling |
| Body size limit | `--max-body 10m` | File uploads fail with `413` by default |
| Forwarded headers | on by default, `--no-forward-headers` to drop | Your app wants to see the visitor's IP and scheme |

With forwarding on, GoNix sends `Host`, `X-Real-IP`, `X-Forwarded-For` and `X-Forwarded-Proto`, so your
application sees the visitor, not `127.0.0.1`.

## WebSockets, with one switch

> **Why does my chat app connect, then drop right away?**
> Because Nginx does not forward the protocol upgrade unless told to.

Without the right headers, a WebSocket handshake fails. With GoNix it is a toggle:

```bash
sudo gonix host set app.example.com --websocket on
```

Behind the scenes it adds `proxy_set_header Upgrade $http_upgrade;`, `proxy_set_header Connection "upgrade";`
and sets `proxy_http_version 1.1` towards the upstream. You never write them by hand.

## Static sites

```bash
sudo gonix host add docs.example.com --root /var/www/docs
```

Serves files from the directory, with `index.html` as the index.

## Redirect hosts

> **What happens to `example.net` when your brand moved to `example.com`?**

```bash
sudo gonix host add example.net --redirect https://example.com --code 301
```

- `--code` is one of `301`, `302`, `307`, `308`. Use `301`/`308` for permanent moves, `302`/`307` for
  temporary ones. `307` and `308` keep the HTTP method, which matters for `POST`.
- By default the path and query survive (`example.net/pricing?x=1` goes to `example.com/pricing?x=1`).
  Add `--no-preserve-path` to send everyone to the target's front page instead.

## Several names, one host

```bash
sudo gonix host add example.com --upstream http://127.0.0.1:3000 \
  --alias www.example.com
```

`--alias` can be repeated. Change the set later with `host set example.com --aliases "www.example.com blog.example.com"`
(or `--aliases none`).

## Extra locations

One domain, several backends:

```bash
sudo gonix host path add example.com /api/ --upstream http://127.0.0.1:4000 --websocket
sudo gonix host path remove example.com /api/
```

A location can have its own IP list and rate limit (`--ip-list`, `--rate-limit`). For anything GoNix
does not model, the TUI offers a **custom location block** where you write raw Nginx directives.

## HTTPS: HTTP/2 and HTTP/3

Give a host a certificate and it listens on 443:

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000 --ssl app.example.com --http3
```

- HTTP to HTTPS redirect is on by default (`--force-https=false` to disable).
- HTTP/2 is on by default, written in the right syntax for your installed Nginx version.
- **HTTP/3 (QUIC)** is offered only when your Nginx build supports it, so you cannot enable something
  that would fail `nginx -t`.

{{< callout type="info" >}}
**Why reference certificates instead of copying them?** See [SSL certificates]({{< relref "ssl" >}}).
Renewing becomes "replace two files".
{{< /callout >}}

## Enable, disable, delete

```bash
sudo gonix host disable app.example.com   # removes the symlink; the file stays
sudo gonix host enable app.example.com
sudo gonix host delete app.example.com --yes   # a backup is kept
```

Deleting needs `--yes` because it is destructive. Even then, a backup remains.

## Changing a host later

`host set` takes the same flags as `host add`, plus toggles such as `--access-log on|off`:

```bash
sudo gonix host set app.example.com --max-body 50m --read-timeout 120s --dry-run
```

Pass `none` to a reference flag (`--ssl none`, `--ip-list none`) to detach it.

Want more? See [Access control]({{< relref "access-control" >}}) and [Traffic shaping]({{< relref "traffic" >}}).
