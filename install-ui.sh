#!/usr/bin/env bash
set -euo pipefail

# PQPM UI Addon Installer (standalone — works on any Linux VPS)
# Usage:
#   curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash
#   sudo ./install-ui.sh [version]
#   sudo ./install-ui.sh --from-source
#   curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash -s -- --with-webmin
#   curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash -s -- --uninstall
#
# Requires core PQPM (pqpm + pqpmd) already installed (except --uninstall).
# Does NOT require Webmin/Virtualmin.

REPO="pqpm/pqpm"
INSTALL_DIR="/usr/local/bin"
UNIT_DST="/etc/systemd/system/pqpm-ui.service"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

FROM_SOURCE=0
WITH_WEBMIN=0
UNINSTALL=0
VERSION=""
for arg in "$@"; do
    case "$arg" in
        --from-source) FROM_SOURCE=1 ;;
        --with-webmin) WITH_WEBMIN=1 ;;
        --uninstall|--remove|--delete) UNINSTALL=1 ;;
        -h|--help)
            cat <<'EOF'
PQPM UI addon installer

Usage:
  sudo ./install-ui.sh [version]
  sudo ./install-ui.sh --from-source [--with-webmin]
  sudo ./install-ui.sh --uninstall

  curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash -s -- --with-webmin
  curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash -s -- --uninstall

Options:
  --from-source   Build pqpm-ui from this git checkout
  --with-webmin   Also install the Webmin/Virtualmin module
  --uninstall     Remove pqpm-ui, systemd unit, and Webmin module
                  (core pqpmd/pqpm are left installed; --with-webmin not needed)
  -h, --help      Show this help
EOF
            exit 0
            ;;
        -*) error "Unknown flag: $arg" ;;
        *) VERSION="$arg" ;;
    esac
done

if [ "$(id -u)" -ne 0 ]; then
    error "This script must be run as root (use sudo)"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || true)"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

webmin_root_dir() {
    local conf="${WEBMIN_CONFIG:-/etc/webmin}/miniserv.conf"
    local dir=""
    if [ -f "$conf" ]; then
        dir=$(awk -F= '/^root=/ { print $2; exit }' "$conf" | tr -d '\r')
    fi
    if [ -n "$dir" ] && [ -d "$dir" ]; then
        echo "$dir"
        return 0
    fi
    for d in /usr/libexec/webmin /usr/share/webmin /opt/webmin /usr/local/webmin; do
        if [ -d "$d" ] && { [ -f "$d/miniserv.pl" ] || [ -f "$d/install-module.pl" ]; }; then
            echo "$d"
            return 0
        fi
    done
    return 1
}

grant_webmin_acl() {
    local acl="${1}/webmin.acl"
    [ -f "$acl" ] || return 0
    local tmp
    tmp=$(mktemp)
    awk '
        BEGIN { granted=0 }
        /^root:/ || /^admin:/ {
            if ($0 !~ /(^| )pqpm( |$)/) { $0 = $0 " pqpm" }
            granted=1
        }
        { print }
        END {
            if (!granted) print "root: pqpm"
        }
    ' "$acl" > "$tmp" && mv "$tmp" "$acl"
}

copy_webmin_module_manual() {
    local src="$1"
    local webmin_dir="$2"
    local webmin_config="$3"
    info "Copying Webmin module into $webmin_dir/pqpm ..."
    install -d "$webmin_dir/pqpm/lang" "$webmin_dir/pqpm/images"
    install -m 0755 "$src"/index.cgi "$src"/action.cgi "$src"/config.cgi "$src"/refresh.cgi "$webmin_dir/pqpm/"
    install -m 0644 "$src"/pqpm-lib.pl "$src"/module.info "$src"/config "$src"/config.info "$src"/install_check.pl "$webmin_dir/pqpm/"
    install -m 0644 "$src"/lang/en "$webmin_dir/pqpm/lang/en"
    if [ -f "$src/images/icon.gif" ]; then
        install -m 0644 "$src/images/icon.gif" "$webmin_dir/pqpm/images/icon.gif"
    fi
    install -d "$webmin_config/pqpm"
    if [ ! -f "$webmin_config/pqpm/config" ]; then
        install -m 0644 "$src/config" "$webmin_config/pqpm/config"
    fi
    grant_webmin_acl "$webmin_config"
}

install_webmin_module() {
    local src="$1"
    if [ "$WITH_WEBMIN" -ne 1 ]; then
        return 0
    fi
    local webmin_config="${WEBMIN_CONFIG:-/etc/webmin}"
    local webmin_dir=""
    webmin_dir=$(webmin_root_dir) || true
    if [ -z "$webmin_dir" ] || [ ! -d "$webmin_dir" ]; then
        warn "Webmin not found; skipping --with-webmin. Standalone pqpm-ui is installed."
        return 0
    fi

    local pack="$TMPDIR/wbm-pack"
    rm -rf "$pack"
    mkdir -p "$pack/pqpm"
    cp -a "$src"/. "$pack/pqpm/"
    chmod 0755 "$pack/pqpm/"*.cgi
    tar -C "$pack" -czf "$TMPDIR/pqpm.wbm.gz" pqpm

    if [ -f "$webmin_dir/install-module.pl" ]; then
        info "Installing Webmin module via $webmin_dir/install-module.pl ..."
        if ! perl "$webmin_dir/install-module.pl" --nodeps "$TMPDIR/pqpm.wbm.gz" "$webmin_config"; then
            warn "install-module.pl failed; falling back to a manual copy"
            copy_webmin_module_manual "$src" "$webmin_dir" "$webmin_config"
        fi
    else
        copy_webmin_module_manual "$src" "$webmin_dir" "$webmin_config"
    fi

    rm -f "$webmin_config/module.infos.cache" "$webmin_config/installed.cache" \
          "$webmin_config/module.infos.cache.json" 2>/dev/null || true
    info "Webmin module installed in $webmin_dir/pqpm"
    info "Then: Webmin → Webmin Configuration → Refresh Modules"
    info "Open: Webmin → Servers → PQPM Process Manager"
    info "(If pqpm is missing, it appears under Un-used Modules until you Refresh Modules.)"
}

revoke_webmin_acl() {
    local acl="${1}/webmin.acl"
    [ -f "$acl" ] || return 0
    local tmp
    tmp=$(mktemp)
    awk '{
        gsub(/(^| )pqpm( |$)/, " ");
        gsub(/  +/, " ");
        sub(/: +/, ": ");
        sub(/ +$/, "");
        print
    }' "$acl" > "$tmp" && mv "$tmp" "$acl"
}

uninstall_webmin_module() {
    local webmin_config="${WEBMIN_CONFIG:-/etc/webmin}"
    local webmin_dir=""
    webmin_dir=$(webmin_root_dir) || true
    if [ -z "$webmin_dir" ]; then
        return 0
    fi
    if [ -d "$webmin_dir/pqpm" ]; then
        info "Removing Webmin module from $webmin_dir/pqpm ..."
        rm -rf "$webmin_dir/pqpm"
    fi
    if [ -d "$webmin_config/pqpm" ]; then
        rm -rf "$webmin_config/pqpm"
    fi
    revoke_webmin_acl "$webmin_config"
    rm -f "$webmin_config/module.infos.cache" "$webmin_config/installed.cache" \
          "$webmin_config/module.infos.cache.json" 2>/dev/null || true
}

uninstall_ui() {
    info "Removing PQPM UI addon (core pqpm is kept)..."
    if command -v systemctl >/dev/null 2>&1; then
        systemctl stop pqpm-ui 2>/dev/null || true
        systemctl disable pqpm-ui 2>/dev/null || true
        systemctl daemon-reload 2>/dev/null || true
    fi
    rm -f "$UNIT_DST" /usr/lib/systemd/system/pqpm-ui.service
    rm -f "$INSTALL_DIR/pqpm-ui"
    uninstall_webmin_module
    info "UI addon removed."
    info "Core PQPM (pqpmd / pqpm) was not uninstalled."
}

if [ "$UNINSTALL" -eq 1 ]; then
    uninstall_ui
    exit 0
fi

command -v pqpm >/dev/null 2>&1 || error "pqpm not found in PATH. Install core PQPM first (install.sh)."
command -v pqpmd >/dev/null 2>&1 || warn "pqpmd not found in PATH; UI needs the daemon running."

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    *) error "Unsupported architecture: $ARCH" ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
[ "$OS" = "linux" ] || error "PQPM UI only supports Linux (detected: $OS)"


install_unit() {
    local src="$1"
    if command -v systemctl >/dev/null 2>&1; then
        info "Installing systemd unit..."
        install -m 0644 "$src" "$UNIT_DST"
        systemctl daemon-reload
        info "Enable with: systemctl enable --now pqpm-ui"
    else
        warn "systemd not found; skip unit install. Run pqpm-ui manually."
    fi
}

USE_SOURCE=0
if [ "$FROM_SOURCE" -eq 1 ]; then
    USE_SOURCE=1
elif [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/addons/ui/cmd/pqpm-ui/main.go" ]; then
    USE_SOURCE=1
fi

if [ "$USE_SOURCE" -eq 1 ]; then
    ROOT="${SCRIPT_DIR:-.}"
    if [ ! -f "$ROOT/addons/ui/cmd/pqpm-ui/main.go" ]; then
        error "Source tree not found. Clone the repo and run: sudo ./install-ui.sh --from-source"
    fi
    info "Building pqpm-ui from source..."
    command -v go >/dev/null 2>&1 || error "Go toolchain required for --from-source"
    (
        cd "$ROOT"
        VERSION_FLAG="${VERSION:-dev}"
        COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
        DATE="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
        LDFLAGS="-s -w -X github.com/pqpm/pqpm/internal/version.Version=${VERSION_FLAG} -X github.com/pqpm/pqpm/internal/version.Commit=${COMMIT} -X github.com/pqpm/pqpm/internal/version.Date=${DATE}"
        go build -trimpath -ldflags "$LDFLAGS" -o "$TMPDIR/pqpm-ui" ./addons/ui/cmd/pqpm-ui
    )
    install -m 0755 "$TMPDIR/pqpm-ui" "$INSTALL_DIR/pqpm-ui"
    install_webmin_module "$ROOT/addons/ui/webmin"
    install_unit "$ROOT/addons/ui/systemd/pqpm-ui.service"
else
    if [ -z "$VERSION" ]; then
        info "Fetching latest release..."
        VERSION=$(curl -sSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | head -n1 | cut -d'"' -f4)
        [ -n "$VERSION" ] || error "Failed to determine latest version"
    fi
    TARBALL="pqpm-ui-${VERSION}-${OS}-${GOARCH}.tar.gz"
    URL="https://github.com/$REPO/releases/download/${VERSION}/${TARBALL}"
    info "Downloading $URL ..."
    if ! curl -fsSL -o "$TMPDIR/$TARBALL" "$URL"; then
        error "Download failed. Build from a git checkout with: sudo ./install-ui.sh --from-source"
    fi
    tar xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"
    [ -f "$TMPDIR/pqpm-ui" ] || error "Archive missing pqpm-ui binary"
    install -m 0755 "$TMPDIR/pqpm-ui" "$INSTALL_DIR/pqpm-ui"
    if [ "$WITH_WEBMIN" -eq 1 ] && [ -d "$TMPDIR/webmin" ]; then
        install_webmin_module "$TMPDIR/webmin"
    fi
    if [ -f "$TMPDIR/pqpm-ui.service" ]; then
        install_unit "$TMPDIR/pqpm-ui.service"
    elif [ -f "$TMPDIR/systemd/pqpm-ui.service" ]; then
        install_unit "$TMPDIR/systemd/pqpm-ui.service"
    fi
fi

info "Installed $INSTALL_DIR/pqpm-ui"
info "Any VPS — standalone UI:"
info "  sudo systemctl enable --now pqpm-ui"
info "  # or: pqpm-ui -listen 127.0.0.1:9090 -auth login"
info "Single-user (no password): pqpm-ui -auth local"
info "Done."
