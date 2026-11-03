---
title: Access control
weight: 60
---

> **Who is allowed to reach this site, and how do you say so without writing `allow`/`deny` rules
> you will misread in six months?**
> GoNix has four independent tools, and they combine.

| Tool | Answers the question | Typical use |
|---|---|---|
| [Access list](#basic-auth-access-lists) | "Do you know the password?" | Staging site, admin panel |
| [IP list](#ip-lists) | "Are you coming from an address I trust or ban?" | Office-only paths, banning an abuser |
| [Country list](#country-blocking) | "Which country are you in?" | Regional restrictions |
| [Real client IP](#real-client-ip) | "What is your *actual* address?" | Anything behind Cloudflare or another proxy |

## Basic Auth access lists

A named, reusable set of users, stored as a standard `htpasswd` file with **bcrypt**-hashed passwords
(plaintext is never stored) at `/etc/gonix/accesslists/<name>.htpasswd`. Apply it to a host or a location and
GoNix adds:

```nginx
auth_basic "Restricted";
auth_basic_user_file /etc/gonix/accesslists/<name>.htpasswd;
```

Use it from the CLI with `host add/set --access-list NAME`, or manage lists and users in the TUI.

### Why is there a "worker group" thing?

Nginx **worker** processes, not the root master, read these files on every request. If they cannot read
them, Nginx answers `500` and logs `open() ".../<name>.htpasswd" failed (13: Permission denied)`.

So at startup GoNix finds the worker group (from `user` in `nginx.conf`, else `nginx -V`: `www-data` on
Debian/Ubuntu, `http` on Arch, `nginx` on RHEL; or `access_lists.group` in `gonix.yaml`) and then:

- gives `/etc/gonix/accesslists/` that group with mode `2750` (setgid, so new files inherit it),
- sets every `*.htpasswd` to `0640` with that group, repairing existing ones,
- adds only the "others execute" bit (`o+x`) to parent directories like `/etc/gonix`, so workers can walk
  through them without listing or reading anything else.

{{< callout type="warning" >}}
bcrypt hashes need an Nginx linked against a libc/crypt that supports bcrypt (modern glibc via libxcrypt,
most current distributions). If yours cannot verify them, regenerate the entries with a tool that emits an
algorithm your `crypt(3)` supports.
{{< /callout >}}

## IP lists

Reusable allow or block lists of IPs and CIDR ranges, attached to a host or a single location.

```bash
sudo gonix iplist create office --mode allow --entries "203.0.113.0/24 198.51.100.7"
sudo gonix host path add example.com /admin/ --upstream http://127.0.0.1:4000 --ip-list office
sudo gonix iplist add office 192.0.2.10        # grows the list; every host using it is updated
```

{{< callout type="info" >}}
**Why named lists instead of rules per host?** The office IP changes once, in one place, and every
host that references the list follows. A list that is still in use cannot be deleted.
{{< /callout >}}

### "From the office *or* with a password"

Combine an IP list with Basic Auth and choose how they relate:

```bash
sudo gonix host set app.example.com --ip-list office --access-list team --satisfy any
```

- `--satisfy any`: either is enough (office visitors skip the password).
- `--satisfy all`: both are required.

## Country blocking

Allow or block countries per host using the `geoip2` Nginx module and a MaxMind database (path in
`geoip.database`).

```bash
sudo gonix country setup
sudo gonix country create eu-only --mode allow --countries "DE FR NL"
sudo gonix host set shop.example.com --country-list eu-only
```

This needs the third-party module (Debian/Ubuntu: `libnginx-mod-http-geoip2`) and a database, for example
from `geoipupdate`.

## Real client IP

> **Why does every request in my logs come from Cloudflare?**
> Because Nginx only sees the connection's address. Behind a proxy, that is the proxy.

That breaks IP lists, rate limits, country lists and logs *at once*: they all see the proxy. Tell GoNix
which proxies to trust, and it restores the visitor's address from the forwarded header:

```bash
sudo gonix realip cloudflare --refresh        # trust Cloudflare, with freshly downloaded ranges
sudo gonix realip custom --proxies "10.0.0.0/8" --header X-Forwarded-For
sudo gonix realip show
sudo gonix realip disable
```

`cloudflare` uses bundled ranges; `--refresh` downloads the current ones.

{{< callout type="warning" >}}
Turn this on **before** relying on IP lists, rate limits or country lists behind a proxy. Otherwise they
act on the proxy's address, and one limit hit blocks everyone.
{{< /callout >}}
