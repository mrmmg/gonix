---
title: Traffic shaping
weight: 70
---

> **Which of the hundred Nginx hardening snippets on the internet should you actually paste?**
> The ones GoNix ships as presets: tested with `nginx -t`, applied with one flag, removed with one flag.

## Presets

```bash
sudo gonix preset list
sudo gonix host set app.example.com --presets security-headers,hsts,gzip
```

| Preset | What it does |
|---|---|
| `security-headers` | `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy` (replacing the upstream's own) |
| `hsts` | Browsers use HTTPS only for this domain for one year (subdomains are not affected) |
| `gzip` | Compress text, JSON, JavaScript, CSS, SVG and fonts |
| `static-cache` | Browsers cache CSS, JavaScript, images, fonts and video for 30 days |
| `proxy-cache` | Cache responses the backend marks cacheable; requests with cookies or `Authorization` always reach the backend |
| `block-sensitive-files` | `404` for dotfiles (`.git`, `.env`, ...) and backup/dump files (`.bak`, `.sql`, ...), except `/.well-known/` |
| `block-bad-bots` | `403` for common scanner user agents (sqlmap, nikto, nuclei, ...) |

{{< callout type="warning" >}}
**HSTS is sticky.** Once a browser has seen it, it refuses plain HTTP for that domain for a year.
Turn it on only when HTTPS works properly.
{{< /callout >}}

`proxy-cache` stores responses in `nginx.cache_dir` (default `/var/cache/nginx/gonix`).

## Rate limiting

> **One misbehaving client hammers `/login` 50 times a second. What now?**
> Give that path a budget.

A rate limit is a named policy applied to a host or a location. Three presets cover common cases:

| Preset | Rate | Burst | Extra |
|---|---|---|---|
| `general` | 30 / second | 60, no delay | up to 50 connections |
| `api` | 10 / second | 20, no delay | |
| `login` | 5 / minute | 5 | |

```bash
sudo gonix ratelimit create login --preset login
sudo gonix host path add app.example.com /login --upstream http://127.0.0.1:3000 --rate-limit login
```

Or define your own:

```bash
sudo gonix ratelimit create uploads --rate 2 --per s --burst 5 --nodelay --conn 10
```

{{< callout type="info" >}}
**Why per-location?** A login page and an image CDN need very different limits. Limiting the whole site
to protect one form punishes everyone. See also [Real client IP]({{< relref "access-control" >}}):
limits count by address.
{{< /callout >}}

## Maintenance mode

> **You need to deploy and break things for ten minutes. Do visitors get a half-broken site?**
> They can get a clean `503` while you still work.

```bash
sudo gonix host set app.example.com --maintenance on --bypass "203.0.113.5"
# ... deploy, test ...
sudo gonix host set app.example.com --maintenance off
```

Everyone gets `503` except the bypass addresses. Combine with a branded
[error page]({{< relref "error-pages" >}}) for 503; GoNix's own error page URIs are exempt, so the custom
page is served.
