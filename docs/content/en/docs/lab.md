---
title: Lab
weight: 185
---

> **How do you try GoNix on hand-written configuration without risking a real server?**
> In the lab: a second Nginx, separate from the system's, that GoNix sets up and fills with test hosts for you.

The lab is hidden unless you ask for it:

```bash
sudo GONIX_LAB=1 gonix
```

The main menu then has a **Lab** item. Without `GONIX_LAB`, it does not exist.

## What the lab is

| Part | Where |
|---|---|
| Nginx configuration | the lab directory (default `/srv/gonix-lab`), with its own `nginx.conf`, `sites-available`, `sites-enabled`, logs and certificates |
| The Nginx process | systemd unit `gonix-lab`, on ports **8080** (HTTP) and **8443** (HTTPS) by default |
| Backends to proxy to | `127.0.0.1:19001` answers "backend A", `127.0.0.1:19002` "backend B" |
| GoNix's view of it | an [instance]({{< relref "instances" >}}) named `lab` (`/etc/gonix/instances/lab.yaml`), with its own backups, history and audit log |

`/etc/nginx` and `nginx.service` are never touched, and the system's Nginx can keep running next to it.

## The Lab menu

- **Set Up Lab:** asks for the directory and the two ports, and whether to add one test host of every kind.
  The directory must be new or empty.
- **Enter Lab:** switches the whole TUI to the lab. The title bar says `lab`, the main menu shows a red
  **LAB** line, and every screen works on the lab's Nginx. **Exit Lab** (or Esc) comes back.
- **Add Test Host:** writes a hand-written host of a chosen kind, enables it, and checks it with `nginx -t`
  (a host that fails is removed again).
- **Start / Stop Lab Nginx**, **Reset Lab** (start over), **Remove Lab** (stops the unit and deletes the
  directory, the unit file and the instance).
- **Usage Guide:** the lab's paths, the commands to use it from the shell, and what to try. It fills the
  terminal; `PgUp` / `PgDn` move a page.

## Test hosts

Each kind is a pattern found on real servers, and each one exercises a different part of
[adopting]({{< relref "migration#adopting-hand-written-hosts-and-certbot" >}}):

| Kind | Suggested domain | What adopting it shows |
|---|---|---|
| Reverse proxy after `certbot --nginx` | `app.lab.test` | certbot's certificate and HTTP block, a custom location |
| Plain reverse proxy | `api.lab.test` | a directive kept as written |
| Static site with `try_files` | `static.lab.test` | a static host, tab indentation kept |
| Whole-domain redirect | `old.lab.test` | a redirect host |
| Wildcard name | `wild.lab.test` | a `*.` name that keeps working |
| PHP, named and WebSocket locations | `php.lab.test` | custom locations next to a proxy |
| Two sites in one file | `multi.lab.test` | a refusal: two server blocks |
| Catch-all default server | | a refusal: `server_name _` |

The certbot host has an imitation certbot directory (`<lab>/letsencrypt`, with a renewal file), so GoNix
treats its certificate as certbot's, and a copy of the certificate in the lab's certificates directory to
[switch to]({{< relref "migration#certificates-from-certbot" >}}). Certificates are self-signed.

## Using it from the shell

The lab is an ordinary instance:

```bash
sudo gonix --instance lab host list
sudo gonix --instance lab adopt --all --dry-run
sudo gonix --instance lab history show <commit>     # the exact diff of one change
```

To send requests to the test domains without editing `/etc/hosts`:

```bash
t() { local h=${1#*://}; h=${h%%/*}; h=${h%%:*}
      curl -sk -i --resolve "$h:8080:127.0.0.1" --resolve "$h:8443:127.0.0.1" "$@"; }
t http://app.lab.test:8080/
t https://app.lab.test:8443/
```

{{< callout type="info" >}}
**Why other ports?** So the lab never competes with the system's Nginx for 80 and 443. GoNix includes the
port in the HTTP-to-HTTPS redirects it writes when HTTPS is not on 443 (the `https_port` setting), so
redirects work in the lab too.
{{< /callout >}}

`adopt --verify-renewal` fails in the lab: its certbot certificates are imitations.
