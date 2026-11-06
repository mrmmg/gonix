---
title: Troubleshooting
weight: 180
---

> **Something does not work. Which symptom is yours?**
> Find it below. Each entry gives the cause, then the fix.

Start with `sudo gonix check`. It catches many of these on its own.

## Authenticated requests return 500

**Cause:** Nginx workers cannot read the `htpasswd` file. The error log says
`open() ".../<name>.htpasswd" failed (13: Permission denied)`.

**Fix:** GoNix repairs ownership and modes at startup. If it guessed the wrong group, set
`access_lists.group` in `gonix.yaml` (`www-data`, `http`, `nginx`...). See
[Access control]({{< relref "access-control" >}}).

## Basic Auth rejects correct passwords

**Cause:** your Nginx's `crypt(3)` cannot verify bcrypt hashes.

**Fix:** regenerate the list's entries with an algorithm it supports.

## My change was rolled back

**Cause:** `nginx -t` failed, so GoNix restored the previous configuration. That is the system working.

**Fix:** read the Nginx error in the output (or the audit log: `sudo gonix audit --lines 5`). Typical reasons:
a missing module (`ngx_http_sub_module` for template error pages, `geoip2`, `stream`), or a missing
certificate file.

## Config Check says nginx.conf does not include conf.d

**Cause:** GoNix writes http-level files into `nginx.http_snippets_dir` (default `/etc/nginx/conf.d`) and
`nginx.conf` has no `include /etc/nginx/conf.d/*.conf;` inside `http {}`.

**Fix:** add that include inside the `http` block, or change `http_snippets_dir` to a directory you do include.

## A host shows as `[unmanaged]`

**Cause:** its file does not start with `# Managed by GoNix.` or `# Adopted by GoNix.`. GoNix will not
rewrite files it did not create.

**Fix:** adopt it (`sudo gonix adopt <domain>`, see [Migration]({{< relref "migration#adopting-hand-written-hosts-and-certbot" >}})),
or keep editing it by hand.

## A host shows as `[adopted, unreadable]`

**Cause:** the adopted file was edited by hand into something GoNix cannot read as one host (for example a
second site, or a syntax error). GoNix leaves it alone; `gonix host show <domain> --json` has the reason in
`adopt_error`.

**Fix:** fix the file by hand, or `sudo gonix adopt release <domain>` to stop managing it.

## All my visitors have the same IP, or one rate-limit hit blocks everyone

**Cause:** you are behind Cloudflare or another proxy, so Nginx sees the proxy's address.

**Fix:** `sudo gonix realip cloudflare --refresh` (or `realip custom ...`).

## HTTP/3 is not offered

**Cause:** your Nginx build does not support QUIC. GoNix offers HTTP/3 only when it does.

**Fix:** upgrade to an Nginx build with HTTP/3.

## Wildcard certificate fails immediately

**Cause:** GoNix verifies the Cloudflare token and zone before running certbot.

**Fix:** check the token has `Zone:DNS:Edit` for that zone and that the domain's DNS is on that account.
`certbot` must also be installed.

## "history" does nothing

**Cause:** `git` is missing, or `history.enabled` is `false`.

**Fix:** install `git`, and check `history.enabled` in `gonix.yaml`.

## `sudo gonix mcp` asks for a password in the agent

**Cause:** `sudo -n` never prompts, so it needs a sudoers rule. See [AI agents]({{< relref "ai-agents" >}}).

## Still stuck?

Open an issue at [github.com/mrmmg/gonix/issues](https://github.com/mrmmg/gonix/issues) with the output of
`gonix --version`, `sudo gonix check --json` and the relevant `sudo gonix audit --lines 20`.
