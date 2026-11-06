---
title: Contributing
weight: 190
---

> **I want to fix a bug or add a feature. Where do I start?**
> Build it, run the tests (no root and no Nginx needed), and read where things live.

## What you need

- Linux, [Go](https://go.dev) **1.26 or newer** (see `go.mod`), and `git`.
- Nginx is optional. Without it the test suite still runs, and the tests that need a real Nginx skip themselves.

## Build from source

### Install it, like the installer would

```bash
git clone https://github.com/mrmmg/gonix.git
cd gonix
make install
```

This is the source-build equivalent of the one-command installer, for when you do not want a prebuilt binary from GitHub Releases. It:

1. Builds the binary with `go build -trimpath -ldflags="-s -w"`.
2. Installs it to `/opt/gonix/bin/gonix` and symlinks `/usr/local/bin/gonix` to it.
3. Installs the default configuration to `/etc/gonix/gonix.yaml`, without overwriting an existing one.
4. Creates `/etc/gonix/{backups,accesslists}` and `/var/log/gonix`.
5. Installs a logrotate policy at `/etc/logrotate.d/gonix` for per-host Nginx logs and the audit log (skipped with a message if `logrotate` is missing).

### Development build

```bash
git clone https://github.com/mrmmg/gonix.git
cd gonix
go mod download

go build -o gonix ./cmd/gonix
sudo ./gonix          # root is required to manage /etc/nginx and systemctl
```

### Running with `go run`

While developing you can skip the build step. `main.go` needs root to touch `/etc/nginx` and `systemctl`, so go through `sudo`, with `sudo -E` if you also want to pass a custom `GONIX_CONFIG`:

```bash
sudo go run ./cmd/gonix
```

The module and build caches normally live under your own user, so the first `sudo go run` in a fresh checkout may build as root once. If you would rather not run `go` itself as root, build a dev binary and point `sudo` at it: `go build -o gonix ./cmd/gonix && sudo ./gonix`.

To try the UI **without touching your real Nginx**, copy `configs/default.yaml`, edit its paths to scratch directories and run:

```bash
sudo GONIX_CONFIG=/path/to/dev-gonix.yaml go run ./cmd/gonix
```

### Production build

```bash
go build -trimpath -ldflags="-s -w" -o gonix ./cmd/gonix
```

- `-trimpath` removes local filesystem paths from the binary, for reproducible, privacy-clean builds.
- `-ldflags="-s -w"` strips the symbol table and DWARF debug information, giving a smaller binary.

### Cross-compilation

```bash
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gonix-linux-amd64 ./cmd/gonix
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o gonix-linux-arm64 ./cmd/gonix
```

This is exactly what [`release.yml`](https://github.com/mrmmg/gonix/blob/main/.github/workflows/release.yml) runs for each architecture. It additionally passes `-X .../internal/tui.Version=<tag>`, so `gonix --version` reports the released version. That is why the asset names match: `gonix-linux-amd64` and `gonix-linux-arm64`.

### Makefile

```bash
make build     # development build
make release   # production build (trimpath + stripped)
make cross     # cross-compile linux/amd64 and linux/arm64
make test      # go test ./...
make vet       # go vet ./...
make lint      # vet + fmt check
make install   # release build + install to /opt/gonix (asks for sudo as needed)
make clean     # remove built binaries
make docs      # build this documentation site
```

## Tests

```bash
go test ./...
go vet ./...
go fmt ./...
```

Tests use temporary directories and fake implementations of external commands (`nginx -t`, `systemctl`), so the full suite runs **without a real Nginx and without root**. No `sudo` is needed for any command above.

Useful variations while you iterate:

```bash
go test ./internal/nginx/...           # just one package
go test ./... -run TestRenderHost -v   # a single test, verbose
go test ./... -cover                   # with a coverage summary
```

When `nginx` is installed, extra tests run every generated configuration through a real `nginx -t`, and some start Nginx on free local ports to check behaviour (redirects, IP lists, rate limits, maintenance, caching). They skip themselves when `nginx` is missing. Stream proxy checks need the stream module, so they run in Docker on request:

```bash
GONIX_DOCKER_TEST=1 go test ./internal/streams/ -run Docker
```

## Project structure

```text
gonix/
├── cmd/gonix/             entry point: wires config, backend services and the TUI together
├── internal/
│   ├── tui/               Bubble Tea screens, navigation and styling; never touches files directly
│   ├── nginx/             domain model (Host/Location/...), template renderer, filesystem
│   │                      operations (sites-available/enabled), `nginx -t` validator, a
│   │                      parser for managed configs, and adopted (hand-written) hosts,
│   │                      edited in place and verified by reading back
│   │   └── ast/           lossless Nginx syntax tree: Print(Parse(x)) == x, byte for byte
│   ├── hosts/             the safe workflow: validate → backup → write → test →
│   │                      reload-or-rollback → audit
│   ├── certificates/      TLS certificate discovery and inspection (crypto/x509), certbot lineages
│   ├── lab/               the Lab: a separate Nginx with hand-written test hosts (GONIX_LAB=1)
│   ├── acme/              wildcard certificates: certbot + Cloudflare API DNS-01
│   ├── accesslist/        HTTP Basic Auth "access lists" backed by htpasswd (bcrypt) files
│   ├── errorpages/        named error page snippets (error_page → <code>.html) included by hosts
│   ├── audit/             append-only audit log
│   ├── backup/            configuration snapshot and restore
│   ├── system/            systemctl control + /proc-based process and port monitoring
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
├── templates/             embedded Nginx templates (host.tmpl, location.tmpl, errorpages.tmpl),
│                          and lab/: the Lab's nginx.conf, unit, guide and one template per test host kind
├── configs/
│   ├── default.yaml       default configuration, also shipped as a release asset
│   └── logrotate.conf     logrotate policy installed by `make install` and install.sh
├── install.sh             one-command installer: downloads a release, no Go toolchain needed
├── Makefile               `make install` builds from source and installs the same layout
├── .github/workflows/
│   ├── release.yml        builds and publishes a GitHub Release on every vX.Y.Z tag push
│   ├── download-count.yml updates the README download badge (on the `badges` branch)
│   └── docs.yml           builds and publishes this documentation to GitHub Pages
├── docs/                  this documentation (a separate Go module, never part of the binary)
└── tests/                 package-local *_test.go files hold the real test suite
```

Responsibilities are layered: the TUI calls into `internal/hosts` (the workflow layer), which calls `internal/nginx` (rendering and filesystem), `internal/backup` and `internal/audit`. **The TUI never writes to `/etc/nginx` directly.**

## Contribution guidelines

- Follow standard Go conventions: `gofmt`, `go vet`, and tests for new behaviour.
- Keep the TUI (`internal/tui`) free of direct filesystem and Nginx access. New backend capability belongs in the matching `internal/*` package and reaches the TUI through `tui.Deps`.
- A change that writes Nginx configuration must go through the shared workflow (backup, `nginx -t`, rollback, audit), so the CLI, the TUI and MCP keep the same guarantees.

## How releases are built

Pushing a version tag (`vX.Y.Z`) triggers the [`Release` workflow](https://github.com/mrmmg/gonix/blob/main/.github/workflows/release.yml). It builds `linux/amd64` and `linux/arm64` and, only if both succeed, publishes a GitHub Release for that exact tag with the binaries, a `checksums.txt` and the default configuration. Nothing is created or edited by hand. The `art/` and `docs/` directories are removed from the build context first, so neither ends up in a release.

A separate workflow (`download-count.yml`) sums the binary download counts and publishes them as the README badge.

## Working on these docs

The documentation is a [Hugo](https://gohugo.io) site using the [Hextra](https://imfing.github.io/hextra/) theme. It needs Hugo **extended** 0.158 or newer.

```bash
cd docs
hugo server       # http://localhost:1313/gonix/
```

Pages are Markdown under `docs/content/en` and `docs/content/fa`. Keep both languages in sync, and give a page the same file name in each. Pushing to `main` publishes the site automatically.
