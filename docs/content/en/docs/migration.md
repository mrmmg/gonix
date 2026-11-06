---
title: Migration
weight: 120
---

> **Moving servers is where configuration goes to be forgotten.**
> GoNix gives you three migration paths, and all of them start with `--dry-run`.

## Moving to a new server

Export everything GoNix manages into one archive, then import it on the other machine.

```bash
sudo gonix export gonix-backup.tar.gz                 # on the old server
sudo gonix import gonix-backup.tar.gz --dry-run       # on the new one: see what would change
sudo gonix import gonix-backup.tar.gz
```

- Export to standard output with `-` instead of a file name, for piping over SSH.
- Import refuses to overwrite existing files unless you add `--overwrite --yes`.

{{< callout type="info" >}}
**Why dry-run an import?** An import is the biggest change you can make in one command. Reading the diff
costs ten seconds and can save the evening.
{{< /callout >}}

## Leaving Nginx Proxy Manager

> **Do you have to recreate thirty hosts by hand?**
> No. Point GoNix at NPM's data directory.

```bash
sudo gonix npm-import --data /opt/npm/data --letsencrypt /opt/npm/letsencrypt --dry-run
```

It brings over proxy hosts, redirects, streams, certificates and access lists. `--disabled` imports the
hosts without enabling them, which is a good first step: look, then enable one by one.

Anything that cannot be mapped (custom advanced configuration, wildcard-only hosts) is **reported instead
of guessed**. A silent wrong guess in a proxy configuration is worse than a visible gap.

## Adopting hand-written hosts (and certbot)

> **Do you have to rewrite sites you configured by hand, or give up certbot, to use GoNix?**
> No. Adopt them: GoNix takes over the file as it is.

```bash
sudo gonix adopt example.com --dry-run    # what GoNix reads from the file, and the one-line change
sudo gonix adopt example.com              # or: sudo gonix adopt --all
```

Adopting adds **one comment** at the top of the file (`# Adopted by GoNix. ...`) and changes nothing else,
so Nginx keeps serving exactly the same configuration. From then on the host is managed: its settings can
be changed from the TUI, the CLI or an AI agent, and each change **edits only the directives involved**.
Comments, ordering and every directive GoNix has no setting for (`try_files`, `fastcgi_pass`, `add_header`,
certbot's own lines, ...) stay exactly as you wrote them.

What GoNix reads from the file:

- server names, listen ports, SSL certificate, HTTP/2, the HTTP-to-HTTPS redirect;
- a `root`, or a server-level `return 301 ...` (a redirect host);
- **locations that are a plain reverse proxy** (`proxy_pass http://host:port` and the usual headers and
  timeouts); every other location is kept as a **custom location**, exactly as written;
- the snippets GoNix itself adds later: presets, IP lists, rate limits, country lists, Basic Auth lists,
  error pages, maintenance mode.

The report lists everything that is **kept as written**, so you can see what GoNix will never touch.

To try all of this on typical hand-written and certbot setups first, use the [Lab]({{< relref "lab" >}}).

**What can be adopted:** a file with one `server` block, optionally plus the plain-HTTP block that redirects
it to HTTPS (the layout `certbot --nginx` writes). The first name in `server_name` must be a plain domain;
wildcards and other names after it are fine. Anything else is skipped with the reason, and stays
`[unmanaged]`.

**Every change is verified before it is written.** After editing, GoNix reads the file back; if the result
is not exactly the host you asked for, the change is refused instead of being written half-applied. Then the
usual path follows: backup, `nginx -t`, reload, or rollback. A file edited by hand since GoNix read it is
never overwritten.

Some changes are refused on adopted hosts, with a message saying so: renaming the host, changing listen
ports, turning SSL off, HTTP/3, and changing the HTTP-to-HTTPS redirect when it lives in a separate server
block. Make those by hand.

### Certificates from certbot

Adopting never touches certificates. A host using `/etc/letsencrypt/live/<name>/fullchain.pem` keeps using
it, and **certbot keeps renewing it** with its timer, as before. GoNix recognizes such certificates
(`gonix host show`, `gonix cert list` mark them as certbot's) and reads certbot's renewal settings to warn
about what renewal needs; for example, with certbot's `nginx` authenticator the host must keep answering
plain HTTP on port 80.

```bash
sudo gonix adopt example.com --verify-renewal   # also runs certbot renew --dry-run for its certificate
```

When you later want GoNix to handle the certificate, put it into the certificates directory (see
[SSL certificates]({{< relref "ssl" >}})) and switch the host. Only the two certificate lines change; HTTP/2
and the redirect stay as they were:

```bash
sudo gonix host set example.com --ssl example.com
```

certbot keeps renewing the old certificate until you remove it (`certbot delete --cert-name example.com`);
GoNix never does that for you.

### Giving a host back

```bash
sudo gonix adopt release example.com
```

removes the marker. The file keeps every change made while GoNix managed it and becomes `[unmanaged]` again.
