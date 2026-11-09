---
title: Install
weight: 10
---

> **Do I really have to compile something, or set up a repository, just to try a tool?**
> No. GoNix is one static binary, and the installer does everything else.

## The one-command install

```bash
curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash
```

The installer detects your architecture, downloads the latest release, installs the binary and
creates a default configuration. Then:

```bash
sudo gonix
```

{{< callout type="info" >}}
**Why `sudo`?** GoNix edits `/etc/nginx` and calls `systemctl`. Both need root. Nothing runs in the
background; when you quit, GoNix is gone.
{{< /callout >}}

### A specific version

```bash
curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash -s -- v1.2.3
```

If your `sudo` password is not cached, the installer asks for it when it needs to write to `/opt`, `/etc` or `/usr/local/bin`.

### Upgrading

Run the installer again, with or without a version. It replaces the binary and tells you what was
installed before. It **never touches** `/etc/gonix`, so your configuration, backups and access lists
survive every upgrade.

Or update from GoNix itself: `sudo gonix update` reads the latest release from GitHub, verifies its
SHA-256 checksum, keeps the current version as `gonix.old` and replaces the binary (`--rollback` puts the
previous one back; `gonix update --check` only looks). The **Updates** menu in the TUI says when a newer
release exists and shows this command. GoNix never updates from anywhere but this project's GitHub Releases.

## What gets installed, and where

| Path                       | Purpose                                                        |
|----------------------------|----------------------------------------------------------------|
| `/opt/gonix/bin/gonix`     | The installed binary                                           |
| `/opt/gonix/VERSION`       | The currently installed version                                |
| `/usr/local/bin/gonix`     | Symlink to the binary, so `gonix` works from any `$PATH`       |
| `/etc/gonix/gonix.yaml`    | Configuration file (created once, never overwritten)           |
| `/etc/gonix/backups/`      | Automatic configuration backups                                |
| `/etc/gonix/accesslists/`  | HTTP Basic Auth access list files                              |
| `/var/log/gonix/audit.log` | Audit log                                                      |
| `/etc/logrotate.d/gonix`   | Log rotation policy (only if `logrotate` is installed)         |

## Supported architectures

| OS    | Architecture | Release asset        |
|-------|--------------|----------------------|
| Linux | amd64        | `gonix-linux-amd64`  |
| Linux | arm64        | `gonix-linux-arm64`  |

## Requirements

- Linux with systemd (GoNix uses `systemctl` and `/proc`), on **amd64** or **arm64**. Any other
  combination is rejected by the installer with a clear error instead of downloading a binary that
  will not run.
- Nginx.
- Optional, each only for one feature: `logrotate` (log rotation), `git` ([configuration history]({{< relref "backups-history" >}})),
  `certbot` ([wildcard certificates]({{< relref "ssl" >}})), the `geoip2` module (country blocking),
  the `stream` module (TCP/UDP proxies).

{{< callout type="warning" >}}
GoNix writes generated http-level files to `/etc/nginx/conf.d`, so your `nginx.conf` must include
that directory inside its `http` block. That is the default on Debian, Ubuntu and RHEL.
[Config Check]({{< relref "logs-and-monitoring" >}}) warns when it does not.
{{< /callout >}}

## Verify a download by hand

If you prefer not to pipe a script into `bash`, every [release](https://github.com/mrmmg/gonix/releases)
has the binaries, a `checksums.txt` and the default `gonix.yaml`:

```bash
curl -fsSLO https://github.com/mrmmg/gonix/releases/latest/download/gonix-linux-amd64
curl -fsSLO https://github.com/mrmmg/gonix/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

Replace `amd64` with `arm64` as needed (the asset names are in the table above). The installer does exactly this and additionally lays out the
directories above, which is why it is the recommended path.

## Building from source instead

See [Contributing]({{< relref "contributing" >}}): `make install` builds with `go build` and installs
into the same layout.
