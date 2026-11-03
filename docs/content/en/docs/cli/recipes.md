---
title: Recipes
weight: 2
---

Short, complete answers to "how do I...". Add `--dry-run` to any of them to preview first.

## Put an app on HTTPS

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000 \
  --ssl app.example.com --presets security-headers,hsts --websocket
```

## Block an abusive client

> **Problem:** one address is hammering your site.

```bash
sudo gonix logs app.example.com --summary          # find the top offender
sudo gonix iplist create blocked --mode block --entries 192.0.2.77
sudo gonix host set app.example.com --ip-list blocked
sudo gonix iplist add blocked 192.0.2.78           # later: add more
```

If the site is behind Cloudflare, run `sudo gonix realip cloudflare` first so the logs show real visitors.

## Restrict `/admin/` to the office

```bash
sudo gonix iplist create office --mode allow --entries "203.0.113.0/24"
sudo gonix host path add app.example.com /admin/ \
  --upstream http://127.0.0.1:4000 --ip-list office
```

## Move a domain, keeping old links alive

```bash
sudo gonix host add old.example.com --alias www.old.example.com \
  --redirect https://new.example.com --ssl old.example.com
```

The path is preserved by default, so `old.example.com/pricing` lands on `new.example.com/pricing`.

## A maintenance window

```bash
sudo gonix host set app.example.com --maintenance on --bypass "203.0.113.5"
# deploy ...
sudo gonix host set app.example.com --maintenance off
```

## Protect a login form from brute force

```bash
sudo gonix ratelimit create login --preset login
sudo gonix host path add app.example.com /login \
  --upstream http://127.0.0.1:3000 --rate-limit login
```

## Something broke, now what?

```bash
sudo gonix check
sudo gonix logs app.example.com --error --lines 50
sudo gonix history list --lines 10
sudo gonix history restore <commit> --yes
```

## Script it

Use `--json` and check the exit code:

```bash
if ! out=$(sudo gonix host add "$domain" --upstream "$url" --json); then
  echo "gonix failed: $out" >&2
  exit 1
fi
```

Label automated changes so the audit log tells people from scripts:

```bash
sudo GONIX_ACTOR=deploy-bot gonix host set app.example.com --max-body 50m
```
