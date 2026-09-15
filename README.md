# GoNix

*[فارسی](README-fa.md)*

**GoNix** — the name comes from **Go** + **Nginx** — is an advanced, interactive **terminal UI
(TUI) for managing Nginx** on Linux, written in Go.

https://github.com/mrmmg/gonix/raw/refs/heads/main/art/gonix_preview.mp4

It is inspired by [Nginx Proxy Manager](https://nginxproxymanager.com/), but takes a different
approach: instead of hiding Nginx behind a database and a proprietary configuration format,
GoNix is a thin, safe management layer **on top of** your real, human-editable Nginx
configuration files. You can always open `/etc/nginx/sites-available/example.com` in a text
editor and see plain, standard Nginx syntax — nothing opaque, nothing proprietary.

The compiled binary is named `gonix`.

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
- **Access Lists (HTTP Basic Auth)** — reusable, bcrypt-hashed `htpasswd`-format user lists that
  can be applied to a host or a specific location.
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

## Requirements

- Linux (uses `systemctl`, `/proc`, and standard Nginx paths)
- [Go](https://go.dev) 1.24 or newer (for building from source)
- Nginx
- `systemctl` (systemd)
- Optional: `logrotate` (log rotation is skipped with a warning if absent)

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
│   ├── accesslist/        HTTP Basic Auth "access lists" backed by htpasswd (bcrypt) files
│   ├── audit/             append-only audit log
│   ├── backup/            configuration snapshot/restore
│   ├── system/            systemctl control + /proc-based process & port monitoring
│   ├── logs/              log path helpers + logrotate config generation
│   └── config/            centralized, YAML-driven configuration (no hard-coded paths)
├── templates/             embedded Nginx config templates (host.tmpl, location.tmpl)
├── configs/
│   ├── default.yaml       default GoNix configuration, also shipped as a release asset
│   └── logrotate.conf     logrotate policy installed by `make install` and install.sh
├── install.sh             one-command installer: downloads a release, no Go toolchain needed
├── Makefile               `make install` builds from source and installs the same layout
├── .github/workflows/
│   └── release.yml        builds + publishes a GitHub Release on every vX.Y.Z tag push
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
(e.g. via `certbot` with a DNS challenge, which is assumed to be run manually/externally) is just
replacing the two files in place — no reconfiguration needed.

> **TODO (future work):** automatic Let's Encrypt/Certbot integration (including DNS challenge
> providers) is intentionally not implemented yet. See `internal/certificates` for where this
> would hook in.

## Access Lists

An access list is a named, reusable set of HTTP Basic Auth users, stored as a standard
`htpasswd`-format file (bcrypt-hashed passwords — plaintext is never stored) under
`/etc/gonix/accesslists/<name>.htpasswd`. Applying a list to a host or location adds:

```nginx
auth_basic "Restricted";
auth_basic_user_file /etc/gonix/accesslists/<name>.htpasswd;
```

> **Note:** bcrypt-hashed htpasswd entries require Nginx to be linked against a libc/crypt
> implementation that supports bcrypt (true for modern glibc via libxcrypt, and for most current
> Linux distributions). If your Nginx build cannot verify bcrypt hashes, regenerate the affected
> access list's entries with a tool that emits an algorithm your `crypt(3)` supports.

## Logs

- **Access/error logs**: per-host, at `/var/log/nginx/<domain>.access.log` and
  `<domain>.error.log`, toggled independently from each host's management screen.
- **Audit log**: `/var/log/gonix/audit.log`, one line per action:
  `2026-09-15 10:32:11 | user=root | action=host_created | target=example.com | result=success`
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

## Development

Contributions should follow standard Go conventions: `gofmt`, `go vet`, and tests for new
behavior. Keep the TUI (`internal/tui`) free of direct filesystem/Nginx access — new backend
capability should be added to the appropriate `internal/*` package and exposed to the TUI through
`tui.Deps`.

## Future Features

Deliberately not implemented yet, but designed for:

- Automatic Let's Encrypt / Certbot integration and DNS challenge providers (e.g. Cloudflare)
- IP allow/deny lists and rate limiting
- Security headers, compression and caching presets
- WAF / ModSecurity integration
- Upstream groups, load balancing and health checks
- HTTP/3, OCSP stapling, HSTS presets
- Maintenance mode and custom error pages
- Importing/diffing arbitrary existing Nginx configuration
- Git-based configuration history and rollback
- Multiple Nginx instance support
