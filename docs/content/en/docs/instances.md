---
title: Multiple instances
weight: 160
---

> **What if one server runs a production Nginx, a staging Nginx and a custom build?**
> One GoNix can manage them all. Each is an *instance*.

Each extra Nginx gets its own configuration file in `/etc/gonix/instances/`:

```bash
sudo gonix instances                              # list them
sudo gonix --instance staging                     # TUI for /etc/gonix/instances/staging.yaml
sudo gonix --instance staging host list           # or a command against it
```

`GONIX_INSTANCE` and `GONIX_CONFIG` work the same as the flags.

## What is separated automatically

Paths left at their defaults (backups, history, audit log, access lists) are **separated per instance**
automatically, so staging never writes into production's backups or history. For example, each instance has
its own `/var/log/gonix/<instance>-audit.log`.

## Pointing at a different Nginx

For an Nginx that is not the system one, set these in the instance file:

```yaml
nginx:
  config_dir: /opt/staging-nginx/etc
  config_file: /opt/staging-nginx/etc/nginx.conf
  service: nginx-staging
```

`nginx.config_file` is passed to `nginx -t -c`, and `nginx.service` is the systemd unit reloaded after
changes. See the [configuration reference]({{< relref "configuration" >}}).
