---
name: gonix
description: Manage Nginx sites, reverse proxies, redirects, SSL, IP/country access rules, rate limits, load balancing, TCP/UDP stream proxies, maintenance mode and logs on a Linux server through the gonix CLI. Use whenever the user asks to add, change, inspect, enable/disable, protect or debug an Nginx host or proxy on a machine where gonix is installed.
---

# Managing Nginx with GoNix

`gonix` is a safety layer over Nginx. Every change it makes is backed up,
validated with `nginx -t`, rolled back automatically if the test fails,
reloaded, written to the audit log and committed to a Git history.

## Rules

1. **Never edit files under /etc/nginx by hand** and never run `systemctl
   reload nginx` yourself for GoNix-managed hosts. Always use `gonix`, so the
   safety checks, audit log and history apply.
2. **Inspect before changing.** Start with `sudo gonix host list --json`
   (and `sudo gonix host show <domain> --json` for details).
3. **Preview every change with `--dry-run`** and show the user the diff.
   Apply it (same command without `--dry-run`) only after they agree.
4. **Destructive commands need `--yes`** (delete, history restore, import
   with overwrite). Only add it when the user explicitly asked for that
   destruction.
5. **Use `--json`** whenever you parse the output. Errors are returned as
   `{"ok": false, "error": "..."}` with a non-zero exit code.
6. Identify yourself: prefix commands with `GONIX_ACTOR=<your-name>` so the
   audit log and history show which changes an agent made.
7. All commands need root: run them with `sudo`.

## Workflow

```bash
sudo gonix host list --json                       # 1. look
sudo gonix host add api.example.com \
     --upstream http://127.0.0.1:3000 --dry-run   # 2. preview (diff)
sudo GONIX_ACTOR=claude gonix host add api.example.com \
     --upstream http://127.0.0.1:3000             # 3. apply after approval
sudo gonix host show api.example.com --json       # 4. verify
sudo gonix check --json                           # 5. look for problems
```

If a change fails, GoNix has already restored the previous configuration.
Report the `nginx -t` output from the error to the user; do not retry with
manual edits.

## Commands

Run `gonix help` for the full, current list (commands marked `*` change the
configuration and accept `--dry-run`).

### Hosts

```bash
gonix host list | show <domain> [--raw]
gonix host add <domain> --upstream http://HOST:PORT [--websocket] [--alias www.<domain>]
gonix host add <domain> --group <upstream-group>
gonix host add <domain> --root /var/www/site
gonix host add <domain> --redirect https://target.com [--code 301|302|307|308] [--no-preserve-path]
    # common: --ssl <cert-name> [--http3] [--force-https=false] --presets a,b
    #         --ip-list N --rate-limit N --country-list N --access-list N --error-pages N --disabled
gonix host set <domain> [same flags] [--aliases "a b"|none] [--websocket on|off]
                        [--maintenance on|off [--bypass "IP ..."]] [--access-log on|off]
                        [--satisfy any|all]   # IP list OR Basic Auth (any) vs both (all)
gonix host enable|disable <domain>
gonix host delete <domain> --yes
gonix host path add <domain> /api/ --upstream http://127.0.0.1:4000 [--ip-list N] [--rate-limit N]
gonix host path remove <domain> /api/
```

`--ssl` takes a certificate directory name under the certificates directory
(`gonix cert list`), e.g. `--ssl example.com` for
`/etc/nginx/certs/example.com/{fullchain,privkey}.pem`. Pass `none` to any
reference flag to clear it.

### Access control and traffic

```bash
gonix iplist list | create <name> --mode allow|block --entries "10.0.0.0/8 1.2.3.4"
gonix iplist add|remove <name> <ip> | delete <name> --yes
gonix ratelimit list | create <name> --preset general|api|login | delete <name> --yes
gonix ratelimit create <name> --rate 5 --per m --burst 5 [--nodelay] [--conn 20]
gonix country list | setup | create <name> --mode allow|block --countries "DE FR"
gonix realip show | cloudflare [--refresh] | custom --proxies "10.0.0.0/8" --header X-Forwarded-For | disable
gonix preset list        # security-headers, hsts, gzip, static-cache, proxy-cache, block-*
```

Behind Cloudflare or a load balancer, enable `realip` first: otherwise IP
lists, rate limits, country lists and logs all see the proxy's address.

### Load balancing and TCP/UDP

```bash
gonix upstream list | create app --servers "10.0.0.11:3000, 10.0.0.12:3000 backup" [--method least_conn]
gonix stream list | setup | create pg --port 5432 --backend 10.0.0.20:5432 [--udp] [--ip-list N]
gonix stream enable|disable <name> | delete <name> --yes
```

### Diagnostics and recovery

```bash
gonix check --json                                 # conflicts, missing files, expiring certs, dead upstreams
gonix logs <domain> --summary --json               # top clients, paths, 5xx
gonix logs <domain> --filter "status:5xx" --lines 20
gonix logs <domain> --error --lines 50
gonix cert list --json
gonix nginx status | test | reload
gonix audit --lines 20 --json
gonix history list --json | show <commit> | restore <commit> --yes
gonix export backup.tar.gz | import backup.tar.gz [--dry-run]
gonix npm-import --data /path/to/npm/data --letsencrypt /path/to/npm/letsencrypt --dry-run
```

## Common tasks

- **Block an abusive client:** find it with `gonix logs <domain> --summary
  --json`, then `gonix iplist create blocked --mode block --entries <ip>` (or
  `gonix iplist add blocked <ip>`) and `gonix host set <domain> --ip-list blocked`.
- **Restrict an admin path to the office:** `gonix iplist create office
  --mode allow --entries "<office range>"`, then add or recreate the path
  with `gonix host path add <domain> /admin/ --upstream ... --ip-list office`.
- **Move a domain:** `gonix host add old.com --alias www.old.com --redirect
  https://new.com --ssl old.com`.
- **Maintenance window:** `gonix host set <domain> --maintenance on --bypass
  "<admin ip>"`, later `--maintenance off`.
- **Something broke:** `gonix check --json`, `gonix logs <domain> --error`,
  and if needed `gonix history list` then `gonix history restore <commit>
  --yes` (with the user's approval).
