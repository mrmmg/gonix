---
title: Configuration reference
weight: 170
---

> **Where does GoNix keep its paths, and can I change them?**
> Everything is in one YAML file, `/etc/gonix/gonix.yaml`, and nothing is hard-coded in the program.

The installer creates it once and **never overwrites** it. Point at another file with `--config PATH` or
the `GONIX_CONFIG` environment variable. The shipped default is
[`configs/default.yaml`](https://github.com/mrmmg/gonix/blob/main/configs/default.yaml).

## `nginx`

| Key | Default | Meaning |
|---|---|---|
| `config_dir` | `/etc/nginx` | Nginx configuration root |
| `sites_available` | `/etc/nginx/sites-available` | Where host files live |
| `sites_enabled` | `/etc/nginx/sites-enabled` | Symlinks for enabled hosts |
| `binary_path` | `nginx` | The Nginx binary |
| `config_file` | `""` | Passed to `nginx -t -c`. Empty uses the binary's default. |
| `service` | `nginx` | systemd unit reloaded after every change |
| `http_snippets_dir` | `/etc/nginx/conf.d` | http-level files (real client IP, rate limit zones, maintenance bypass). `nginx.conf` must include it inside `http`. |
| `cache_dir` | `/var/cache/nginx/gonix` | Where the `proxy-cache` preset stores responses |
| `streams_dir` | `/etc/nginx/gonix-streams` | TCP/UDP stream proxies |
| `main_include_dir` | `/etc/nginx/modules-enabled` | Top-level include dir where GoNix can create a `stream` block |

## `certificates`

| Key | Default | Meaning |
|---|---|---|
| `directory` | `/etc/nginx/certs` | One folder per domain with `fullchain.pem` and `privkey.pem` |

## `logs`

| Key | Default | Meaning |
|---|---|---|
| `nginx_directory` | `/var/log/nginx` | Per-host access and error logs |
| `audit_file` | `/var/log/gonix/audit.log` | The audit log |

## `backup`

| Key | Default | Meaning |
|---|---|---|
| `directory` | `/etc/gonix/backups` | Timestamped copies before every write |
| `keep_count` | `20` | Snapshots kept per file; older ones are pruned |

## `access_lists`

| Key | Default | Meaning |
|---|---|---|
| `directory` | `/etc/gonix/accesslists` | Basic Auth `htpasswd` files |
| `group` | `""` | Group that owns them so workers can read. Empty means auto-detect. |

## `error_pages`

| Key | Default | Meaning |
|---|---|---|
| `snippets_directory` | `/etc/nginx/snippets` | Generated `gonix-error-pages-<name>.conf` files |
| `pages_directory` | `/var/www/html/error_pages` | Default directory offered for the HTML pages |

## `geoip`

| Key | Default | Meaning |
|---|---|---|
| `database` | `/usr/share/GeoIP/GeoLite2-Country.mmdb` | MaxMind country database for country blocking |

## `history`

| Key | Default | Meaning |
|---|---|---|
| `enabled` | `true` | Commit every successful change to Git |
| `git_dir` | `/etc/gonix/history.git` | Repository, kept outside `/etc/nginx` |

## Environment variables

| Variable | Meaning |
|---|---|
| `GONIX_CONFIG` | Configuration file to use |
| `GONIX_INSTANCE` | Instance to manage (same as `--instance`) |
| `GONIX_ACTOR` | Name recorded as the actor in the audit log |
