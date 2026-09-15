#!/usr/bin/env bash
#
# gonix installer (build from source).
#
# Builds (if needed) and installs gonix to /opt/gonix (symlinked from
# /usr/local/bin), its configuration file, and a logrotate policy for its
# audit log. Run as root:
#
#   sudo ./scripts/install.sh
#
# This script is for building and installing from a local checkout. To
# install a prebuilt release without cloning the repository or needing a Go
# toolchain, use the top-level install.sh instead (see README.md):
#
#   curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash
#
set -euo pipefail

BIN_NAME="gonix"
INSTALL_DIR="/opt/gonix"
BIN_DIR="${INSTALL_DIR}/bin"
GLOBAL_BIN_DIR="${INSTALL_PREFIX:-/usr/local/bin}"
CONFIG_DIR="/etc/gonix"
CONFIG_FILE="${CONFIG_DIR}/gonix.yaml"
LOGROTATE_FILE="/etc/logrotate.d/gonix"
BACKUP_DIR="${CONFIG_DIR}/backups"
ACCESSLIST_DIR="${CONFIG_DIR}/accesslists"
AUDIT_LOG_DIR="/var/log/gonix"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[1;33m==> warning:\033[0m %s\n' "$1"; }
die()  { printf '\033[1;31m==> error:\033[0m %s\n' "$1" >&2; exit 1; }

require_root() {
    if [[ "$(id -u)" -ne 0 ]]; then
        die "this installer must be run as root, e.g.: sudo $0"
    fi
}

detect_os() {
    if [[ ! -f /etc/os-release ]]; then
        warn "cannot detect operating system (missing /etc/os-release); continuing anyway"
        return
    fi
    # shellcheck disable=SC1091
    . /etc/os-release
    log "detected OS: ${PRETTY_NAME:-unknown}"
}

check_dependencies() {
    local missing=()
    for cmd in systemctl; do
        command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
    done
    if ! command -v nginx >/dev/null 2>&1; then
        warn "nginx binary not found in PATH; install and configure Nginx before using gonix"
    fi
    if ! command -v logrotate >/dev/null 2>&1; then
        warn "logrotate not found; log rotation for /var/log/nginx and the audit log will not run automatically"
    fi
    if [[ ${#missing[@]} -gt 0 ]]; then
        die "missing required system utilities: ${missing[*]}"
    fi
}

build_binary() {
    if [[ -f "${REPO_ROOT}/${BIN_NAME}" ]]; then
        log "using existing prebuilt binary ${REPO_ROOT}/${BIN_NAME}"
        return
    fi
    command -v go >/dev/null 2>&1 || die "Go toolchain not found; install Go or place a prebuilt '${BIN_NAME}' binary in the repository root"
    log "building ${BIN_NAME} from source"
    local version
    version="$(cat "${REPO_ROOT}/VERSION" 2>/dev/null || echo dev)"
    (cd "$REPO_ROOT" && go build -trimpath \
        -ldflags="-s -w -X github.com/mrmmg/gonix/internal/tui.Version=${version}" \
        -o "$BIN_NAME" ./cmd/gonix)
}

install_binary() {
    log "installing binary to ${BIN_DIR}/${BIN_NAME}"
    mkdir -p "$BIN_DIR"
    install -Dm755 "${REPO_ROOT}/${BIN_NAME}" "${BIN_DIR}/${BIN_NAME}"
    cat "${REPO_ROOT}/VERSION" 2>/dev/null > "${INSTALL_DIR}/VERSION" || true

    log "linking global command: ${GLOBAL_BIN_DIR}/${BIN_NAME}"
    mkdir -p "$GLOBAL_BIN_DIR"
    ln -sf "${BIN_DIR}/${BIN_NAME}" "${GLOBAL_BIN_DIR}/${BIN_NAME}"
}

install_config() {
    mkdir -p "$CONFIG_DIR" "$BACKUP_DIR" "$ACCESSLIST_DIR" "$AUDIT_LOG_DIR"
    if [[ -f "$CONFIG_FILE" ]]; then
        log "configuration file already exists at ${CONFIG_FILE}, leaving it untouched"
    else
        log "installing default configuration to ${CONFIG_FILE}"
        install -Dm644 "${REPO_ROOT}/configs/default.yaml" "$CONFIG_FILE"
    fi
    chmod 750 "$CONFIG_DIR" "$BACKUP_DIR" "$ACCESSLIST_DIR"
}

install_logrotate() {
    if ! command -v logrotate >/dev/null 2>&1; then
        return
    fi
    log "installing logrotate policy to ${LOGROTATE_FILE}"
    cat > "$LOGROTATE_FILE" <<'EOF'
/var/log/nginx/*.access.log /var/log/nginx/*.error.log {
    daily
    rotate 14
    missingok
    notifempty
    compress
    delaycompress
    sharedscripts
    postrotate
        [ -f /var/run/nginx.pid ] && kill -USR1 $(cat /var/run/nginx.pid)
    endscript
}

/var/log/gonix/audit.log {
    weekly
    rotate 12
    missingok
    notifempty
    compress
    delaycompress
}
EOF
}

main() {
    require_root
    detect_os
    check_dependencies
    build_binary
    install_binary
    install_config
    install_logrotate

    log "installation complete."
    log "Run gonix with: sudo ${BIN_NAME}"
}

main "$@"
