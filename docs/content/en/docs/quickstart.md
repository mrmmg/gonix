---
title: Quickstart
weight: 20
---

> **How long until I have a real HTTPS site behind Nginx?**
> About five minutes, and you will see every line GoNix writes along the way.

Assume you run an app on `127.0.0.1:3000` and want `app.example.com` to reach it over HTTPS.

{{% steps %}}

### Put a certificate in place

GoNix expects certificates in one central directory, one folder per domain:

```text
/etc/nginx/certs/app.example.com/fullchain.pem
/etc/nginx/certs/app.example.com/privkey.pem
```

If you do not have one yet, GoNix can issue a wildcard certificate for you; see
[SSL certificates]({{< relref "ssl" >}}).

### Preview the host

Add `--dry-run` and GoNix shows the exact files it would write, as a diff, without touching anything:

```bash
sudo gonix host add app.example.com \
  --upstream http://127.0.0.1:3000 \
  --ssl app.example.com \
  --presets security-headers,hsts \
  --dry-run
```

### Apply it

Run the same command without `--dry-run`. Behind the scenes GoNix backs up, writes, runs `nginx -t`
and reloads only if the test passed.

### Look at what it made

```bash
sudo gonix host show app.example.com --raw
```

You are looking at ordinary Nginx configuration in `/etc/nginx/sites-available/app.example.com`:

```nginx
# Managed by GoNix. Manual edits are preserved but will not be
# tracked by the application unless the host is re-imported.
# Domain: app.example.com

server {
    listen 80;
    listen 443 ssl;
    server_name app.example.com;

    if ($scheme != "https") {
        return 301 https://$host$request_uri;
    }

    ssl_certificate /etc/nginx/certs/app.example.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/app.example.com/privkey.pem;
    # include lines for the two presets, access/error log, and the proxy location ...
}
```

Plain HTTP is redirected to HTTPS by default (`--force-https=false` turns that off).

### Check for mistakes

```bash
sudo gonix check
```

Config Check looks for duplicate server names, missing files, expiring certificates and unreachable
upstreams.

{{% /steps %}}

## Prefer menus?

Run `sudo gonix` and choose **Add New Host**. It is a guided wizard for the same options (upstream,
protocol, timeouts, WebSocket, SSL, presets, access lists), and everything it does goes through the
same safety path as the commands above.

{{< callout type="info" >}}
**Why two interfaces?** Menus are the fastest way to *explore* what is possible. Commands are the
fastest way to *repeat* it, in scripts, in CI, or by an AI agent. Both call the same code, so they
can never disagree.
{{< /callout >}}

## Where next

- Why GoNix works this way: [How it works]({{< relref "how-it-works" >}})
- More host types (static sites, redirects, WebSockets): [Hosts]({{< relref "hosts" >}})
