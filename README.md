<img src="art/logo-full.jpeg" alt="GoNix Logo" width="50%" />

# GoNix

[![Latest Release](https://img.shields.io/github/v/release/mrmmg/gonix?label=release)](https://github.com/mrmmg/gonix/releases/latest)
[![Binary Downloads](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmrmmg%2Fgonix%2Fbadges%2Fdownloads.json&cacheSeconds=3600)](https://github.com/mrmmg/gonix/releases)
[![License](https://img.shields.io/github/license/mrmmg/gonix)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/mrmmg/gonix)](go.mod)

*[فارسی](README-fa.md)*

**GoNix** — the name comes from **Go** + **Nginx** — is an advanced, interactive **terminal UI
(TUI) for managing Nginx** on Linux, written in Go.

It is inspired by [Nginx Proxy Manager](https://nginxproxymanager.com/), but takes a different
approach: instead of hiding Nginx behind a database and a proprietary configuration format,
GoNix is a thin, safe management layer **on top of** your real, human-editable Nginx
configuration files. You can always open `/etc/nginx/sites-available/example.com` in a text
editor and see plain, standard Nginx syntax — nothing opaque, nothing proprietary.

The compiled binary is named `gonix`.

**Video preview:**

<p align="center">

https://github.com/user-attachments/assets/45f5a47a-ebe0-4b5c-a6c1-e2c971aaa8d5

</p>

## Table of Contents

- [Installation](#installation)
  - [Manual download](#manual-download)
  - [Supported architectures](#supported-architectures)
  - [What gets installed, and where](#what-gets-installed-and-where)
  - [Building and installing from source instead](#building-and-installing-from-source-instead)
  - [How releases are built](#how-releases-are-built)
- [Features](#features)
- [Requirements](#requirements)
- [Building from Source](#building-from-source)
  - [Install from source with `make install`](#install-from-source-with-make-install)
  - [Development build](#development-build)
  - [Running with `go run` (development)](#running-with-go-run-development)
  - [Production build](#production-build)
  - [Cross-compilation](#cross-compilation)
  - [Makefile](#makefile)
- [Development Commands](#development-commands)
- [Project Structure](#project-structure)
- [Nginx Configuration](#nginx-configuration)
- [SSL Certificates](#ssl-certificates)
  - [Get Wildcard Certificate (Certbot)](#get-wildcard-certificate-certbot)
    - [Renewing a certificate](#renewing-a-certificate)
- [Access Lists](#access-lists)
- [Error Pages](#error-pages)
- [Logs](#logs)
- [Backups](#backups)
- [Command Line (CLI)](#command-line-cli)
- [AI Agents (Skill & MCP)](#ai-agents-skill--mcp)
- [Multiple Nginx Instances](#multiple-nginx-instances)
- [Configuration History, Export & Import](#configuration-history-export--import)
- [Development](#development)
- [Future Features](#future-features)
- [Star History](#star-history)
- [License](#license)

## Installation

Install the latest version of GoNix with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash
```

That's it. The installer automatically detects your architecture, downloads the latest release,
installs GoNix, and sets up the default configuration. Once it finishes, run:

```bash
sudo gonix
```

To install a specific version instead of the latest:

```bash
curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash -s -- v1.2.3
```

Running the installer again — with or without a version argument — upgrades an existing
installation: it replaces the `gonix` binary with the requested version and reports what was
previously installed, but never touches your existing configuration in `/etc/gonix`.

If you don't already have `sudo` privileges cached, the installer will prompt for your password
when it needs to write to `/opt`, `/etc`, or `/usr/local/bin`.

### Manual download

Prebuilt binaries, a `checksums.txt`, and the default `gonix.yaml` are attached to every
[GitHub Release](https://github.com/mrmmg/gonix/releases). To download and verify a binary by
hand (replace `amd64` with `arm64` as needed):

```bash
curl -fsSLO https://github.com/mrmmg/gonix/releases/latest/download/gonix-linux-amd64
curl -fsSLO https://github.com/mrmmg/gonix/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

The installer above does the same download and additionally sets up the directory layout below,
so it is the recommended path for most users.

### Supported architectures

| OS    | Architecture | Release asset          |
|-------|--------------|-------------------------|
| Linux | amd64        | `gonix-linux-amd64`    |
| Linux | arm64        | `gonix-linux-arm64`    |

Any other OS/architecture combination is rejected by the installer with a clear error rather than
downloading something that won't run.

### What gets installed, and where

| Path                          | Purpose                                                        |
|--------------------------------|----------------------------------------------------------------|
| `/opt/gonix/bin/gonix`         | The installed binary                                           |
| `/opt/gonix/VERSION`           | The currently installed version                                |
| `/usr/local/bin/gonix`         | Symlink to the binary above, so `gonix` works from any `$PATH` |
| `/etc/gonix/gonix.yaml`        | Configuration file (created once, never overwritten)           |
| `/etc/gonix/backups/`          | Automatic configuration backups                                |
| `/etc/gonix/accesslists/`      | HTTP Basic Auth access list files                               |
| `/var/log/gonix/audit.log`     | Audit log                                                       |
| `/etc/logrotate.d/gonix`       | Log rotation policy (if `logrotate` is installed)               |

### Building and installing from source instead

If you'd rather build locally (no GitHub release download), see
[Building from Source](#building-from-source) below — `make install` builds the binary with
`go build` and installs it into the same `/opt/gonix` layout described above.

### How releases are built

Pushing a version tag (`vX.Y.Z`, e.g. `v1.2.3`) to this repository triggers the
[`Release` GitHub Actions workflow](.github/workflows/release.yml), which builds `linux/amd64` and
`linux/arm64` binaries, and — only if both builds succeed — publishes a GitHub Release for that
exact tag containing the binaries, a `checksums.txt`, and the default configuration. Nothing about
a release is created or edited by hand.

## Features

- **Host management** — create, list, enable/disable, and delete virtual hosts, named after their
  domain (`sites-available/example.com`, not an opaque ID), linked into `sites-enabled` the
  standard Nginx way.
- **Reverse proxy & static hosts** — guided setup for upstream host/port, protocol, HTTP version,
  timeouts, body size limits, and forwarded headers.
- **Locations** — add additional reverse-proxy locations to a host, or a fully custom location
  block for advanced cases.
- **WebSocket support** — a single Yes/No toggle configures the right `Upgrade`/`Connection`/
  `proxy_http_version` directives; you never write them by hand.
- **SSL/TLS** — references certificates from a centralized directory
  (`/etc/nginx/certs/<domain>/{fullchain,privkey}.pem`) rather than copying them per host, so
  renewing a certificate is just replacing two files. RSA and ECDSA are both supported and
  detected automatically.
- **Certificate inspection** — expiration (human-readable and exact), issuer, subject, SANs,
  SHA-256 fingerprint, key/cert matching, and chain verification, all using Go's `crypto/x509`
  (no `openssl` subprocess required).
- **Wildcard certificates** — issue and renew `*.domain.com` certificates with `certbot` and a
  Cloudflare DNS-01 challenge, fully automated from the TUI (see
  [Get Wildcard Certificate (Certbot)](#get-wildcard-certificate-certbot)).
- **Access Lists (HTTP Basic Auth)** — reusable, bcrypt-hashed `htpasswd`-format user lists that
  can be applied to a host or a specific location.
- **Error pages** — reusable, named snippets mapping HTTP error codes (e.g. 502, 503, 504) to your
  own HTML pages, included by any host with one menu choice.
- **Access/error logs** — per-host enable/disable, with logrotate configured on install.
- **Audit log** — every change made through the application is recorded with timestamp, Linux
  user, action, target and result.
- **Nginx service control & monitoring** — start/stop/restart/reload/test, with master/worker
  process discovery, memory usage, uptime and listening ports read directly from `/proc`.
- **Safety first** — every configuration change is backed up, written, tested with `nginx -t`,
  and only reloaded if the test passes. A failed test automatically restores the previous,
  working configuration and records the failure in the audit log. Nginx is never left broken.
- **Coexists with hand-written configuration** — hosts not generated by GoNix are shown
  read-only (with a clear `[unmanaged]` marker) instead of being silently rewritten.
- **Redirect hosts & domain aliases** — redirect a whole domain (e.g. `example.net` →
  `https://example.com`) with 301/302/307/308, keeping or dropping the path, and serve several
  names (`www.example.com`) from one host.
- **IP access lists** — reusable allow/block lists of IPs and CIDR ranges, attached to a host or a
  single location, and combinable with Basic Auth ("IP *or* password" via `satisfy any`).
- **Real client IP** — trust Cloudflare (bundled or freshly downloaded ranges) or your own proxies,
  so logs, IP lists and rate limits see the visitor's address instead of the proxy's.
- **Host presets** — one-click security headers, HSTS, gzip, static asset caching, proxy caching,
  sensitive file blocking (`.env`, `.git`, ...) and bad bot blocking.
- **Rate limiting** — named request/connection limits (with `general`, `api` and `login`
  presets) applied per host or per location.
- **Maintenance mode** — serve a 503 page for a host while letting your own IPs through.
- **HTTP/2 and HTTP/3** — the right `http2` syntax for the installed Nginx version; HTTP/3 (QUIC)
  is offered only when the Nginx build supports it.
- **Live logs** — follow a host's requests with filters (`status:5xx`, `ip:`, `path:`), a
  summary of top IPs/paths/statuses, and blocking an abusive IP straight from the log view.
- **Config Check** — finds duplicate server names, missing files, expiring or mismatched
  certificates, unreachable upstreams, unused snippets and other common mistakes.
- **Upstream groups** — load balancing (round robin, least connections, IP hash) with weights,
  backup servers and passive health checks.
- **Stream (TCP/UDP) proxies** — forward databases, game servers or any TCP/UDP port, optionally
  behind an IP list.
- **Country blocking** — allow or block countries per host with the geoip2 module and a MaxMind
  database.
- **Configuration history** — every successful change is committed to a Git repository kept
  outside `/etc/nginx`; browse diffs and restore any earlier state.
- **Export / import** — move everything GoNix manages to another server in one archive.
- **Nginx Proxy Manager import** — bring proxy hosts, redirects, streams, certificates and access
  lists over from an existing NPM installation.
- **Command line & AI agents** — every action is also a scriptable command with `--json` and
  `--dry-run`, plus a bundled agent skill and an MCP server, so tools like Claude Code can manage
  Nginx safely.
- **Multiple Nginx instances** — manage several Nginx installations from one GoNix with
  `--instance`.

## Requirements

- Linux (uses `systemctl`, `/proc`, and standard Nginx paths)
- [Go](https://go.dev) 1.26 or newer (for building from source; see `go.mod`)
- Nginx
- `systemctl` (systemd)
- Optional: `logrotate` (log rotation is skipped with a warning if absent)
- Optional: `git`, for [configuration history](#configuration-history-export--import)
- Optional: the geoip2 Nginx module (Debian/Ubuntu: `libnginx-mod-http-geoip2`) and a MaxMind
  country database, only for country blocking
- Optional: the stream module (Debian/Ubuntu: `libnginx-mod-stream`), only for TCP/UDP proxies
- Generated http-level files go to `/etc/nginx/conf.d` (`nginx.http_snippets_dir`), which
  `nginx.conf` must include inside its `http` block — the default on Debian, Ubuntu and RHEL.
  Config Check warns when it does not.
- Optional: `certbot`, only needed for the [Get Wildcard Certificate (Certbot)](#get-wildcard-certificate-certbot)
  feature (with a domain managed by Cloudflare)

## Building from Source

### Install from source with `make install`

```bash
git clone https://github.com/mrmmg/gonix.git
cd gonix
make install
```

This is the source-build equivalent of the one-command installer above, useful when you don't
want to fetch a prebuilt binary from GitHub Releases. It will:

1. Build the binary from source (via `go build -trimpath -ldflags="-s -w"`).
2. Install it to `/opt/gonix/bin/gonix` and symlink `/usr/local/bin/gonix` to it.
3. Install the default configuration to `/etc/gonix/gonix.yaml` (without
   overwriting an existing one).
4. Create `/etc/gonix/{backups,accesslists}` and `/var/log/gonix`.
5. Install a logrotate policy at `/etc/logrotate.d/gonix` covering both per-host Nginx
   logs and the audit log (skipped with a message if `logrotate` isn't installed).

Afterwards:

```bash
sudo gonix
```

### Development build

```bash
git clone https://github.com/mrmmg/gonix.git
cd gonix

go mod download

# Development build
go build -o gonix ./cmd/gonix

# Run locally (root is required to manage /etc/nginx and systemctl)
sudo ./gonix
```

### Running with `go run` (development)

While developing, you can skip the explicit build step and run straight from source with
`go run`. Since `main.go` requires root to touch `/etc/nginx` and `systemctl`, invoke it via
`sudo` (using `sudo -E` if you also want to pass through a custom `GONIX_CONFIG`):

```bash
sudo go run ./cmd/gonix
```

Because the module cache and build cache normally live under your own user, the first `sudo go
run` in a fresh checkout may need to build as root once; if you'd rather not run `go` itself as
root, point `sudo` at a prebuilt dev binary instead (`go build -o gonix ./cmd/gonix && sudo
./gonix`).

To point it at a throwaway configuration instead of the real `/etc/nginx` (handy for trying out
the UI without touching your actual Nginx setup), copy `configs/default.yaml`, edit its paths to
some local scratch directories, and run:

```bash
sudo GONIX_CONFIG=/path/to/dev-gonix.yaml go run ./cmd/gonix
```

### Production build

```bash
go build -trimpath -ldflags="-s -w" -o gonix ./cmd/gonix
```

- `-trimpath` removes local filesystem paths from the compiled binary, for reproducible,
  privacy-clean builds.
- `-ldflags="-s -w"` strips the symbol table and DWARF debugging information, producing a
  smaller binary.

### Cross-compilation

```bash
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gonix-linux-amd64 ./cmd/gonix
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o gonix-linux-arm64 ./cmd/gonix
```

This is exactly what [`.github/workflows/release.yml`](.github/workflows/release.yml) runs for
each architecture (additionally passing `-X .../internal/tui.Version=<tag>` so `gonix --version`
reports the released version), which is why the asset names match: `gonix-linux-amd64` and
`gonix-linux-arm64`.

### Makefile

A `Makefile` wraps the commands above:

```bash
make build     # development build
make release   # production build (trimpath + stripped)
make cross     # cross-compile linux/amd64 and linux/arm64
make test      # go test ./...
make vet       # go vet ./...
make lint      # vet + fmt check
make install   # release build + install to /opt/gonix (prompts for sudo as needed)
make clean     # remove built binaries
```

## Development Commands

```bash
go test ./...
go vet ./...
go fmt ./...
```

Tests use temporary directories and fake implementations of external commands (`nginx -t`,
`systemctl`), so the full suite runs without a real Nginx installation or root privileges — no
`sudo` needed for any of the commands above.

Useful variations while iterating on a single package:

```bash
go test ./internal/nginx/...           # just one package
go test ./... -run TestRenderHost -v   # a single test, verbose
go test ./... -cover                   # with coverage summary
```

When `nginx` is installed, extra tests run every generated configuration through a real
`nginx -t`, and some start Nginx on free local ports to check behaviour (redirects, IP lists,
rate limits, maintenance, caching). They are skipped automatically when `nginx` is missing.
Stream proxy checks need the stream module, so they run in Docker on request:

```bash
GONIX_DOCKER_TEST=1 go test ./internal/streams/ -run Docker
```

## Project Structure

```text
gonix/
├── cmd/gonix/             entry point: wires config, backend services and the TUI together
├── internal/
│   ├── tui/               Bubble Tea screens, navigation, and styling — never touches files directly
│   ├── nginx/             domain model (Host/Location/...), template renderer, filesystem
│   │                      operations (sites-available/enabled), `nginx -t` validator, and a
│   │                      best-effort parser for both managed and hand-written configs
│   ├── hosts/             the safe, high-level workflow: validate → backup → write → test →
│   │                      reload-or-rollback → audit
│   ├── certificates/      TLS certificate discovery and inspection (crypto/x509)
│   ├── acme/              automated wildcard certificates: certbot + Cloudflare API DNS-01
│   │                      (see "Get Wildcard Certificate (Certbot)" under SSL Certificates)
│   ├── accesslist/        HTTP Basic Auth "access lists" backed by htpasswd (bcrypt) files
│   ├── errorpages/        named error page snippets (error_page → <code>.html) included by hosts
│   ├── audit/             append-only audit log
│   ├── backup/            configuration snapshot/restore
│   ├── system/            systemctl control + /proc-based process & port monitoring
│   ├── logs/              log path helpers, logrotate config, access log parsing for live logs
│   ├── config/            centralized, YAML-driven configuration (no hard-coded paths)
│   ├── snippet/           writes a set of generated files with one nginx -t, rollback and dry-run
│   ├── iplist/ realip/ presets/ ratelimit/ maintenance/ geoblock/
│   │                      one package per reusable snippet feature
│   ├── upstreams/ streams/ health/
│   │                      upstream groups, TCP/UDP proxies and upstream reachability checks
│   ├── lint/              Config Check
│   ├── history/ transfer/ npm/ diff/
│   │                      Git history, export/import, NPM import, unified diffs
│   ├── app/               builds every service from the configuration (shared by TUI and CLI)
│   ├── cli/               non-interactive commands and the MCP server
│   └── nginxtest/         test helper that runs a real nginx on free ports
├── skills/gonix/          the AI agent skill (SKILL.md), embedded in the binary
├── templates/             embedded Nginx config templates (host.tmpl, location.tmpl,
│                          errorpages.tmpl)
├── configs/
│   ├── default.yaml       default GoNix configuration, also shipped as a release asset
│   └── logrotate.conf     logrotate policy installed by `make install` and install.sh
├── install.sh             one-command installer: downloads a release, no Go toolchain needed
├── Makefile               `make install` builds from source and installs the same layout
├── .github/workflows/
│   ├── release.yml        builds + publishes a GitHub Release on every vX.Y.Z tag push
│   └── download-count.yml updates the README binary download badge (on the `badges` branch)
└── tests/                 (package-local *_test.go files hold the actual test suite)
```

Responsibilities are layered: the TUI calls into `internal/hosts` (the workflow layer), which
calls into `internal/nginx` (rendering + filesystem), `internal/backup` and `internal/audit`. The
TUI never writes to `/etc/nginx` directly.

## Nginx Configuration

Hosts are stored the standard Nginx way:

```text
/etc/nginx/sites-available/example.com   # always present, the source of truth
/etc/nginx/sites-enabled/example.com     # symlink to sites-available, present only when enabled
```

Disabling a host removes only the symlink; the configuration file itself is never deleted.
Configuration files generated by GoNix start with a `# Managed by GoNix.` comment,
which is how the application distinguishes them from hand-written files on subsequent scans.
Hosts without that marker are listed as **[unmanaged]** and shown read-only in the TUI — you can
still view their raw configuration, but editing them is left to a text editor to avoid corrupting
configuration GoNix didn't create.

## SSL Certificates

Certificates are expected in a centralized directory (configurable, default
`/etc/nginx/certs`):

```text
/etc/nginx/certs/
├── example.com/
│   ├── fullchain.pem
│   └── privkey.pem
└── example.org/
    ├── fullchain.pem
    └── privkey.pem
```

Generated host configuration references these files directly:

```nginx
ssl_certificate /etc/nginx/certs/example.com/fullchain.pem;
ssl_certificate_key /etc/nginx/certs/example.com/privkey.pem;
```

GoNix does **not** copy certificates into per-host directories, so renewing a certificate
(e.g. via `certbot` with a DNS challenge, run manually/externally, or via the automated flow
below) is just replacing the two files in place — no reconfiguration needed.

### Get Wildcard Certificate (Certbot)

From the **Certificates** screen, the first entry — **"Get Wildcard Certificate (Certbot)"** —
automates issuing a wildcard certificate (`*.domain.com` + `domain.com`) end to end using
`certbot`'s DNS-01 challenge and the **Cloudflare API**. This is currently the only supported DNS
provider.

Normally, `certbot certonly --manual --preferred-challenges dns` requires a human in the loop: it
prints a TXT record to create, waits for you to set it in your DNS panel, and asks you to press
Enter once you've confirmed it propagated (e.g. with `dig`). GoNix replaces that interactive step
with certbot's own `--manual-auth-hook`/`--manual-cleanup-hook` mechanism — certbot runs a hook
command itself and waits for it to finish, so no terminal interaction is needed. The hook is this
same `gonix` binary invoked as `gonix acme-hook auth|cleanup` (a hidden subcommand; see
`internal/acme`), which:

1. Creates the `_acme-challenge.domain.com` TXT record via the Cloudflare API.
2. Polls public DNS directly (bypassing local caches) until the record is visible, replacing the
   manual `dig`-and-press-Enter step.
3. Lets certbot validate the challenge and issue the certificate.
4. Removes the TXT record afterwards via the Cloudflare API (the cleanup hook).

The wizard asks for three things:

| Field                | Notes                                                                 |
|-----------------------|------------------------------------------------------------------------|
| Base domain           | Without the leading `*.`, e.g. `example.com`                          |
| Cloudflare API Token  | Needs `Zone:DNS:Edit` permission for the domain's zone; input is masked |
| Contact email         | Optional; used for Let's Encrypt renewal notices                      |

Before running `certbot` at all, GoNix verifies the Cloudflare token and confirms the domain's
zone exists in that Cloudflare account, so a bad token or a domain that isn't on Cloudflare fails
immediately with a clear message instead of after certbot has already started.

On success, GoNix copies the issued `fullchain.pem`/`privkey.pem` from certbot's
`/etc/letsencrypt/live/<domain>/` into the centralized certificates directory
(`<certificates.directory>/<domain>/`, see [SSL Certificates](#ssl-certificates) above), chmod'd
`0644` so Nginx can always read them, exactly like a manually placed certificate. This works
whether certbot just issued a brand new certificate, or — as it does by default when a certificate
isn't due for renewal yet — left the existing one (an `/etc/letsencrypt/archive/` entry and its
`live/` symlinks) untouched: either way, whatever is currently in `live/` gets installed.

**Requirements:** `certbot` must already be installed on the server (GoNix does not install it),
and the domain's DNS must be managed by the Cloudflare account the API token belongs to. Every run
is recorded in the [audit log](#logs) as `wildcard_certificate_issued`.

**Live progress, not a frozen screen:** most of the time this takes is spent waiting for the TXT
record to propagate on the public DNS — this can be a few minutes. Rather than leaving the UI
looking hung, GoNix streams certbot's own output live as it happens, plus a periodic heartbeat
line, so you always see that something is happening; the full log stays on screen (scrollable with
`↑↓`) once the run finishes, whether it succeeded or failed.

#### Renewing a certificate

Selecting an existing certificate from the **Certificates** screen opens a small submenu with
**"View Details"** and **"Renew Certificate (Certbot + Cloudflare)"**. Renewing:

- Only asks for the Cloudflare API token again (and, optionally, an email) — never for the domain,
  which is already known, and never stores the token between runs.
- Passes `--force-renewal` to certbot, so it's unconditional once confirmed (unlike a fresh "Get
  Wildcard Certificate" run, which leaves an unexpired certificate alone by default).
- Warns first if the certificate still has more than 30 days of validity left — matching certbot
  and Let's Encrypt's own default renewal threshold — since renewing earlier than that only burns
  into Let's Encrypt's rate limits for no benefit. You can still confirm and renew anyway (e.g. if
  you just changed something about the domain's DNS setup and want to verify the flow works).
- On success, also re-runs `nginx -t` and reloads Nginx automatically: the certificate's file path
  under the certificates directory doesn't change on renewal, so any host already configured to use
  it only needs Nginx to reread the (now updated) file, not a configuration change.

## Access Lists

An access list is a named, reusable set of HTTP Basic Auth users, stored as a standard
`htpasswd`-format file (bcrypt-hashed passwords — plaintext is never stored) under
`/etc/gonix/accesslists/<name>.htpasswd`. Applying a list to a host or location adds:

```nginx
auth_basic "Restricted";
auth_basic_user_file /etc/gonix/accesslists/<name>.htpasswd;
```

Nginx worker processes (not the root master process) read these files on every request, so they
must be readable by the worker group. On startup GoNix detects that group — from the `user`
directive in `nginx.conf`, else from `nginx -V` (`www-data` on Debian/Ubuntu, `http` on Arch,
`nginx` on RHEL) — or uses `access_lists.group` from `/etc/gonix/gonix.yaml` when set. It then:

- gives `/etc/gonix/accesslists/` that group with mode `2750` (setgid, so new files inherit it),
- sets every `*.htpasswd` file to mode `0640` with that group, repairing existing ones,
- adds only the "others execute" bit (`o+x`) to parent directories such as `/etc/gonix`, so
  workers can traverse them without listing or reading anything else in them.

Without this, Nginx answers authenticated requests with **500** and logs
`open() ".../<name>.htpasswd" failed (13: Permission denied)`.

> **Note:** bcrypt-hashed htpasswd entries require Nginx to be linked against a libc/crypt
> implementation that supports bcrypt (true for modern glibc via libxcrypt, and for most current
> Linux distributions). If your Nginx build cannot verify bcrypt hashes, regenerate the affected
> access list's entries with a tool that emits an algorithm your `crypt(3)` supports.

## Error Pages

An error pages snippet is a named, reusable Nginx snippet that answers selected HTTP error codes
with your own static HTML pages. Each snippet stores **one** pages directory and uses one of two
modes:

- **Separate page per code** — one file per code named `<code>.html` (e.g. `502.html`).
- **Single template** — one shared file (e.g. `error.html`) in which every occurrence of a
  placeholder (default `ERROR_CODE_PLACEHOLDER`) is replaced with the status code.

Create and edit snippets from **Main Menu → Error Pages**; the default directory offered is
`error_pages.pages_directory` in `/etc/gonix/gonix.yaml` (`/var/www/html/error_pages`).

In per-code mode, a snippet named `default` handling 502 and 504 is written to
`/etc/nginx/snippets/gonix-error-pages-default.conf`:

```nginx
error_page 502 /__gonix_error_pages/502.html;
error_page 504 /__gonix_error_pages/504.html;

location ^~ /__gonix_error_pages/ {
    internal;
    auth_basic off;
    alias /var/www/html/error_pages/;
}
```

In template mode, a snippet named `branded` is written to
`/etc/nginx/snippets/gonix-error-pages-branded.conf`:

```nginx
error_page 400 401 403 404 413 429 500 502 503 504 /__gonix_error_page;

location = /__gonix_error_page {
    internal;
    auth_basic off;
    root /var/www/html/error_pages;
    try_files /error.html =500;
    types { }
    default_type text/html;
    sub_filter 'ERROR_CODE_PLACEHOLDER' $status;
    sub_filter_once off;
}
```

`$status` holds the original error code, so a single location serves every code, and the
response keeps that status (e.g. 502). Template mode requires Nginx built with
`ngx_http_sub_module` (`nginx -V 2>&1 | grep -o http_sub_module`). Without it, `nginx -t` fails
and GoNix rolls the change back. GoNix warns when the template file does not contain the
placeholder.

Choosing a snippet for a host (in **Add New Host**, or a host's **Error Pages** menu) adds a single line
at server level:

```nginx
include /etc/nginx/snippets/gonix-error-pages-default.conf;
```

- Changing a snippet's codes or directory updates every host that includes it. Like every
  other change, it is tested with `nginx -t` and rolled back if the test fails.
- A snippet cannot be deleted while a host still uses it.
- `auth_basic off` keeps error pages viewable on hosts protected by an access list.
- Pages are served from an internal location, so they cannot be requested directly, and assets
  must be inlined (CSS in `<style>`, images as `data:` URIs).
- Only errors Nginx itself returns are replaced, e.g. 502/504 when the upstream is down or slow.
  Error responses produced by the upstream application pass through unchanged.
- A missing page file does not break the configuration; GoNix warns about it and shows each
  page's status on the snippet's screen.
- A **disabled** host has no server block at all, so its requests are handled by Nginx's default
  server, not by its error pages.

## Logs

- **Access/error logs**: per-host, at `/var/log/nginx/<domain>.access.log` and
  `<domain>.error.log`, toggled independently from each host's management screen.
- **Audit log**: `/var/log/gonix/audit.log`, one line per action:
  `2026-09-15 10:32:11 | user=root | action=host_created | target=example.com | result=success`.
  Changes made from the command line add `| actor=cli` (or the value of `GONIX_ACTOR`), and
  each extra instance has its own `/var/log/gonix/<instance>-audit.log`.
- **Rotation**: both are rotated by standard `logrotate`, configured automatically by either
  installer at `/etc/logrotate.d/gonix`.

## Backups

Before GoNix writes any change to a host's configuration file, it saves a timestamped
copy under `/etc/gonix/backups/`. After writing, it runs `nginx -t`:

- **Test passes** → Nginx is reloaded, the change is recorded as a success in the audit log.
- **Test fails** → the previous configuration is automatically restored from the backup just
  taken, Nginx is **never** reloaded with broken configuration, and the failure (with the Nginx
  error output) is recorded in the audit log.

Old backups beyond the configured `backup.keep_count` (default 20 per file) are pruned
automatically. The "Backup / Restore" screen on each host lets you list and manually restore any
retained snapshot, following the same test-before-reload safety path.

## Command Line (CLI)

Run `gonix` with no arguments for the TUI, or with a command for scripts, CI and AI agents:

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000 --ssl app.example.com --presets security-headers,hsts
sudo gonix host add example.net --redirect https://example.com --code 301
sudo gonix iplist create office --mode allow --entries "203.0.113.0/24 198.51.100.7"
sudo gonix host set app.example.com --ip-list office --dry-run   # show the diff, change nothing
sudo gonix logs app.example.com --filter "status:5xx" --summary
sudo gonix check --json
```

- `--json` prints machine-readable results; errors are `{"ok":false,"error":"..."}` with exit
  code 1 (usage errors exit with 2).
- `--dry-run` shows the exact files that would change, as a diff, without touching anything.
- Destructive commands (delete, restore, overwrite) require `--yes`.
- Every change goes through the same backup → `nginx -t` → reload-or-rollback path as the TUI.

`gonix help` lists every command, and `gonix help <command>` only the matching ones.

## AI Agents (Skill & MCP)

GoNix ships with an agent skill that teaches AI coding agents how to use the CLI safely
(inspect first, `--dry-run` before changes, never edit `/etc/nginx` by hand):

```bash
gonix skill install            # Claude Code, for your user (~/.claude/skills/gonix)
gonix skill install --project  # only for the current project (./.claude/skills/gonix)
gonix skill print              # print SKILL.md for any other agent
```

It also includes an [MCP](https://modelcontextprotocol.io) server on stdio, whose typed tools map
to the CLI commands:

```bash
claude mcp add gonix -- sudo -n gonix mcp
```

`sudo -n` needs a sudoers rule that lets your user run `gonix` without a password. Changes made
by agents are recorded in the audit log with `actor=mcp`.

## Multiple Nginx Instances

Each extra Nginx installation gets its own configuration file in `/etc/gonix/instances/`:

```bash
sudo gonix instances                              # list them
sudo gonix --instance staging                     # open the TUI for /etc/gonix/instances/staging.yaml
sudo gonix --instance staging host list           # or run a command against it
```

Paths left at their defaults (backups, history, audit log, access lists) are separated per
instance automatically. Set `nginx.config_file` and `nginx.service` for an Nginx that is not the
system one. `GONIX_INSTANCE` and `GONIX_CONFIG` work the same as the flags.

## Configuration History, Export & Import

When `history.enabled` is on (the default) and `git` is installed, every successful change is
committed to a Git repository at `/etc/gonix/history.git`, with `/etc/nginx` as its work tree —
nothing is added inside `/etc/nginx`. "Configuration History" in the TUI, or
`gonix history list|show|restore`, shows each change as a diff and restores any earlier state
(tested with `nginx -t` first, like everything else).

To move to a new server:

```bash
sudo gonix export gonix-backup.tar.gz                 # on the old server
sudo gonix import gonix-backup.tar.gz --dry-run       # on the new one: see what would change
sudo gonix import gonix-backup.tar.gz
```

To migrate from Nginx Proxy Manager, point GoNix at NPM's data directory:

```bash
sudo gonix npm-import --data /opt/npm/data --letsencrypt /opt/npm/letsencrypt --dry-run
```

Anything that cannot be mapped (custom advanced configuration, wildcard-only hosts) is reported
instead of being guessed.

## Development

Contributions should follow standard Go conventions: `gofmt`, `go vet`, and tests for new
behavior. Keep the TUI (`internal/tui`) free of direct filesystem/Nginx access — new backend
capability should be added to the appropriate `internal/*` package and exposed to the TUI through
`tui.Deps`.

## Future Features

Deliberately not implemented yet, but designed for:

- Additional DNS challenge providers for wildcard certificates (only Cloudflare is supported today)
- WAF / ModSecurity integration
- Active health checks (open-source Nginx only has passive ones)
- Importing arbitrary hand-written Nginx configuration into managed hosts

OCSP stapling is intentionally left out: Let's Encrypt stopped running OCSP responders in 2025.

## Star History

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=mrmmg/gonix&type=Date&theme=dark" />
  <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=mrmmg/gonix&type=Date" />
</picture>

## License

GoNix is released under the [MIT License](LICENSE).
