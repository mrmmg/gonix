---
title: SSL certificates
weight: 50
---

> **Why does renewing a certificate usually mean editing every site that uses it?**
> It does not have to. GoNix is designed so it never does.

## One folder, one rule

GoNix expects certificates in one central directory (default `/etc/nginx/certs`), one folder per domain:

```text
/etc/nginx/certs/
├── example.com/
│   ├── fullchain.pem
│   └── privkey.pem
└── example.org/
    ├── fullchain.pem
    └── privkey.pem
```

A host then *points at* those files:

```nginx
ssl_certificate /etc/nginx/certs/example.com/fullchain.pem;
ssl_certificate_key /etc/nginx/certs/example.com/privkey.pem;
```

{{< callout type="info" >}}
**Why not copy certificates into each host?** Because then renewal means touching every host. With a
shared path, renewing is "replace two files". The path never changes, and no configuration needs editing.
{{< /callout >}}

List what you have:

```bash
sudo gonix cert list
```

## GoNix's, Let's Encrypt's and custom certificates

GoNix lists certificates from two places, and always says whose each one is:

| Tag | Where it lives | Who renews it |
|---|---|---|
| `[GoNix]` | `certificates.directory` (default `/etc/nginx/certs`) | GoNix (wildcard flow below) |
| `[Let's Encrypt]` | certbot's `live/` folder (`certificates.letsencrypt_dir`, default `/etc/letsencrypt`) | certbot |
| `[Custom]` | any other path a host file already points at (for example an adopted host) | whoever put it there |

The tag appears after the domain in the **Certificates** screen, in the certificate choice of **Add New Host**
and **SSL Configuration**, and on the **SSL Configuration** line of a host (for example
`[Let's Encrypt] 42d left`). Choosing a Let's Encrypt certificate points the host at certbot's own
`live/<name>/` files, so certbot's renewals keep working with no copying. A custom certificate is offered as
"kept as it is", so editing a host never silently swaps it for another one.

{{< callout type="warning" >}}
**Red means act.** A certificate that is expired, unreadable or has **14 days or less** left is shown in red
with a ⚠ in the list, on the host's SSL line, and as a notice on the **main menu** naming each certificate.
{{< /callout >}}

## Inspecting a certificate

In the TUI's **Certificates** screen, **View Details** shows expiry (human-readable and exact), issuer,
subject, SANs, SHA-256 fingerprint, whether key and certificate match, and chain verification. It is all
done with Go's `crypto/x509`, with no `openssl` subprocess. RSA and ECDSA are both detected automatically.

## Getting a wildcard certificate

> **Do I have to run certbot by hand, create a TXT record in a DNS panel, wait, and press Enter?**
> Normally yes. GoNix removes the human from the loop.

From **Certificates → Get Wildcard Certificate (Certbot)** GoNix issues `*.domain.com` plus `domain.com`
using certbot's DNS-01 challenge and the **Cloudflare API** (currently the only supported DNS provider).

### The problem it solves

`certbot certonly --manual --preferred-challenges dns` prints a TXT record, waits until *you* have created
it (and checked with `dig`) and asks you to press Enter. GoNix swaps that manual step for certbot's own
`--manual-auth-hook` and `--manual-cleanup-hook`: certbot runs a command and waits for it. That command is
the same `gonix` binary, run as a hidden subcommand (`gonix acme-hook auth|cleanup`), which:

1. Creates the `_acme-challenge.domain.com` TXT record through the Cloudflare API.
2. Polls public DNS directly (bypassing local caches) until the record is visible, replacing the
   manual `dig` and Enter.
3. Lets certbot validate and issue the certificate.
4. Removes the TXT record afterwards (the cleanup hook).

### What it asks you

| Field | Notes |
|---|---|
| Base domain | Without the leading `*.`, e.g. `example.com` |
| Cloudflare API token | Needs `Zone:DNS:Edit` for the domain's zone. Input is masked. |
| Contact email | Optional. Used by Let's Encrypt for renewal notices. |

Before running certbot at all, GoNix verifies the token and confirms the zone exists in that account, so
a bad token or a domain that is not on Cloudflare fails **immediately** with a clear message, not after
certbot has been running for minutes.

### While it runs

Most of the time goes to waiting for the TXT record to propagate, which can take a few minutes. GoNix
streams certbot's output live, plus a heartbeat line, so the screen never looks frozen. The full log stays
scrollable (`↑↓`) after the run, whether it succeeded or failed.

### The result

GoNix copies `fullchain.pem` and `privkey.pem` from `/etc/letsencrypt/live/<domain>/` into
`<certificates.directory>/<domain>/` (mode `0644`, so Nginx can always read them), exactly like a
manually placed certificate. This works whether certbot just issued a new certificate or left an
unexpired one alone (an `/etc/letsencrypt/archive/` entry and its `live/` symlinks untouched): whatever is in `live/` gets installed. The run is recorded in the audit log as
`wildcard_certificate_issued`.

{{< callout type="warning" >}}
`certbot` must already be installed (GoNix does not install it), and the domain's DNS must be managed by
the Cloudflare account the token belongs to.
{{< /callout >}}

## Renewing

Select a certificate in **Certificates** and choose **Renew Certificate (Certbot + Cloudflare)**.

- It asks only for the Cloudflare token (and optionally an email). The domain is already known, and the
  token is **never stored** between runs.
- It passes `--force-renewal`, so renewal is unconditional once confirmed.
- If more than 30 days of validity remain, it warns first. That is certbot's and Let's Encrypt's own
  threshold, and renewing earlier only burns rate limits. You can still confirm, for example to test a DNS change.
- On success it re-runs `nginx -t` and reloads Nginx. The path did not change, so hosts need no edit, only a
  re-read of the updated file.

### Let's Encrypt certificates (certbot)

When `certbot` is installed, selecting a `[Let's Encrypt]` certificate offers **Renew Certificate (certbot)**.
It runs `certbot renew --cert-name <name>` with the output shown live, then tests and reloads Nginx. certbot
skips a certificate that is not due, so a certificate with more than 30 days left first asks whether to force
the renewal. If certbot is not found, the screen says so. The `[Let's Encrypt]` certificate is renewed with
whatever settings it was issued with (nginx plugin, webroot, DNS, ...); GoNix does not ask for a Cloudflare
token for it.

{{< callout type="info" >}}
**Using another way to get certificates?** Anything that puts `fullchain.pem` and `privkey.pem` in
`<certificates.directory>/<domain>/` works. GoNix does not care how they got there.
{{< /callout >}}
