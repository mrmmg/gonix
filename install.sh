#!/usr/bin/env bash
#
# One-command installer for GoNix.
#
#   curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash
#
# Installs a specific version instead of the latest:
#
#   curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash -s -- v1.2.3
#
# Downloads a prebuilt release binary from GitHub Releases — no Go
# toolchain or repository checkout required. For building from source
# instead, see scripts/install.sh in the repository.
set -euo pipefail

# Paths are overridable via environment variables, mainly so this script can
# be exercised in CI/tests without needing real root or touching a real
# machine's /opt and /etc. Normal end users should never need to set these.
REPO="${GONIX_REPO:-mrmmg/gonix}"
INSTALL_DIR="${GONIX_INSTALL_DIR:-/opt/gonix}"
BIN_DIR="${INSTALL_DIR}/bin"
CONFIG_DIR="${GONIX_CONFIG_DIR:-/etc/gonix}"
CONFIG_FILE="${CONFIG_DIR}/gonix.yaml"
BACKUP_DIR="${CONFIG_DIR}/backups"
ACCESSLIST_DIR="${CONFIG_DIR}/accesslists"
AUDIT_LOG_DIR="${GONIX_LOG_DIR:-/var/log/gonix}"
LOGROTATE_FILE="${GONIX_LOGROTATE_FILE:-/etc/logrotate.d/gonix}"
GLOBAL_BIN="${GONIX_GLOBAL_BIN:-/usr/local/bin/gonix}"

REQUESTED_VERSION="${1:-latest}"

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------
c_blue="\033[1;34m"; c_green="\033[1;32m"; c_yellow="\033[1;33m"; c_red="\033[1;31m"; c_reset="\033[0m"

step()  { printf "\n${c_blue}==>${c_reset} %s\n" "$1"; }
info()  { printf "${c_blue}==>${c_reset} %s\n" "$1"; }
ok()    { printf "${c_green}==>${c_reset} %s\n" "$1"; }
warn()  { printf "${c_yellow}==> warning:${c_reset} %b\n" "$1" >&2; }
die()   { printf "${c_red}==> error:${c_reset} %b\n" "$1" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Privilege handling: installing to /opt, /etc and /usr/local/bin requires
# root. Rather than forcing `curl ... | sudo bash` (which the caller may not
# expect), run as the current user and prefix privileged commands with sudo
# when not already root.
# ---------------------------------------------------------------------------
SUDO=""
if [ "${GONIX_NO_SUDO:-0}" != "1" ] && [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
    else
        die "This installer must be run as root, or with sudo available.\n       Try: curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sudo bash"
    fi
fi

# ---------------------------------------------------------------------------
# 1-3. Detect OS, architecture and required tools.
# ---------------------------------------------------------------------------
step "Detecting system architecture..."

os="$(uname -s)"
if [ "$os" != "Linux" ]; then
    die "Unsupported operating system: ${os}. GoNix currently supports Linux only."
fi

arch_raw="$(uname -m)"
case "$arch_raw" in
    x86_64|amd64)
        arch="amd64"
        ;;
    aarch64|arm64)
        arch="arm64"
        ;;
    *)
        die "Unsupported CPU architecture: ${arch_raw}. Supported architectures: linux/amd64, linux/arm64."
        ;;
esac

info "Detected: linux/${arch}"

for tool in curl install mkdir uname; do
    command -v "$tool" >/dev/null 2>&1 || die "Required tool '${tool}' was not found.\n       Install it with your distribution's package manager and re-run this script."
done

have_sha256sum=1
command -v sha256sum >/dev/null 2>&1 || have_sha256sum=0

# ---------------------------------------------------------------------------
# 4. Resolve the release to install.
# ---------------------------------------------------------------------------
step "Fetching Gonix release information..."

if [ "$REQUESTED_VERSION" = "latest" ]; then
    api_response="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")" \
        || die "Could not reach GitHub to determine the latest release. Check your network connection."
    version="$(printf '%s' "$api_response" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')"
    [ -n "$version" ] || die "Could not determine the latest release version from the GitHub API response."
else
    version="$REQUESTED_VERSION"
    [[ "$version" == v* ]] || version="v${version}"
fi

info "Target version: ${version}"

# ---------------------------------------------------------------------------
# Detect an existing installation for upgrade reporting.
# ---------------------------------------------------------------------------
previous_version=""
if [ -x "$GLOBAL_BIN" ]; then
    previous_version="$("$GLOBAL_BIN" --version 2>/dev/null | awk '{print $NF}' || true)"
fi
if [ -n "$previous_version" ]; then
    info "Currently installed: v${previous_version}"
    info "Upgrading to:        ${version}"
else
    info "No existing installation detected. Installing ${version}."
fi

# ---------------------------------------------------------------------------
# 5. Download the correct binary (and checksums, best-effort).
# ---------------------------------------------------------------------------
step "Downloading Gonix..."

release_base="https://github.com/${REPO}/releases/download/${version}"
asset_name="gonix-linux-${arch}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL -o "${tmpdir}/${asset_name}" "${release_base}/${asset_name}" \
    || die "Failed to download ${asset_name} for release ${version}.\n       Check that the release exists at: https://github.com/${REPO}/releases/tag/${version}"

if [ "$have_sha256sum" -eq 1 ]; then
    if curl -fsSL -o "${tmpdir}/checksums.txt" "${release_base}/checksums.txt" 2>/dev/null; then
        expected="$(grep "  ${asset_name}\$" "${tmpdir}/checksums.txt" | awk '{print $1}')"
        actual="$(sha256sum "${tmpdir}/${asset_name}" | awk '{print $1}')"
        if [ -n "$expected" ] && [ "$expected" != "$actual" ]; then
            die "Checksum verification failed for ${asset_name}. The download may be corrupted or tampered with."
        elif [ -n "$expected" ]; then
            info "Checksum verified."
        else
            warn "Could not find a checksum entry for ${asset_name}; skipping verification."
        fi
    else
        warn "Could not download checksums.txt; skipping verification."
    fi
else
    warn "sha256sum not found; skipping checksum verification."
fi

chmod +x "${tmpdir}/${asset_name}"

# ---------------------------------------------------------------------------
# 6-8. Install the binary, directories, and permissions.
# ---------------------------------------------------------------------------
step "Installing Gonix to ${INSTALL_DIR}..."

$SUDO mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$BACKUP_DIR" "$ACCESSLIST_DIR" "$AUDIT_LOG_DIR"
$SUDO chmod 750 "$CONFIG_DIR" "$BACKUP_DIR" "$ACCESSLIST_DIR"
$SUDO install -m 0755 "${tmpdir}/${asset_name}" "${BIN_DIR}/gonix"
$SUDO sh -c "echo '${version#v}' > '${INSTALL_DIR}/VERSION'"

# ---------------------------------------------------------------------------
# Default configuration: never overwrite an existing file.
# ---------------------------------------------------------------------------
step "Installing default configuration..."

if [ -f "$CONFIG_FILE" ]; then
    ok "Existing configuration preserved: ${CONFIG_FILE}"
else
    config_downloaded=0
    if curl -fsSL -o "${tmpdir}/gonix.yaml" "${release_base}/gonix.yaml" 2>/dev/null; then
        config_downloaded=1
    fi
    if [ "$config_downloaded" -eq 1 ]; then
        $SUDO install -m 0640 "${tmpdir}/gonix.yaml" "$CONFIG_FILE"
    else
        warn "Could not download the default configuration from the release; writing built-in defaults instead."
        $SUDO sh -c "cat > '${CONFIG_FILE}'" <<'EOF'
nginx:
  config_dir: /etc/nginx
  sites_available: /etc/nginx/sites-available
  sites_enabled: /etc/nginx/sites-enabled
  binary_path: nginx

certificates:
  directory: /etc/nginx/certs

logs:
  nginx_directory: /var/log/nginx
  audit_file: /var/log/gonix/audit.log

backup:
  directory: /etc/gonix/backups
  keep_count: 20

access_lists:
  directory: /etc/gonix/accesslists
EOF
        $SUDO chmod 0640 "$CONFIG_FILE"
    fi
    ok "Configuration created: ${CONFIG_FILE}"
fi

# ---------------------------------------------------------------------------
# logrotate policy (idempotent; safe to overwrite, it is not user-editable
# state).
# ---------------------------------------------------------------------------
if command -v logrotate >/dev/null 2>&1; then
    $SUDO mkdir -p "$(dirname "$LOGROTATE_FILE")"
    $SUDO sh -c "cat > '${LOGROTATE_FILE}'" <<'EOF'
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
    ok "Logrotate policy installed: ${LOGROTATE_FILE}"
else
    warn "logrotate not found; skipping log rotation setup (Nginx and audit logs will grow unbounded until logrotate is installed)."
fi

# ---------------------------------------------------------------------------
# 9. Global command.
# ---------------------------------------------------------------------------
step "Creating global command: ${GLOBAL_BIN}"
$SUDO mkdir -p "$(dirname "$GLOBAL_BIN")"
$SUDO ln -sf "${BIN_DIR}/gonix" "$GLOBAL_BIN"

# ---------------------------------------------------------------------------
# Verify.
# ---------------------------------------------------------------------------
step "Verifying installation..."
if installed_version="$("$GLOBAL_BIN" --version 2>/dev/null)"; then
    ok "Installed: ${installed_version}"
else
    die "Installation verification failed: '${GLOBAL_BIN} --version' did not run successfully."
fi

echo
ok "Installation completed successfully!"
echo
echo "You can now run:"
echo
echo "    gonix"
echo
echo "GoNix requires root to manage Nginx, so start it with:"
echo
echo "    sudo gonix"
echo
