#!/usr/bin/env bash
set -euo pipefail

# PQPM UI Addon Installer (standalone — works on any Linux VPS)
# Usage:
#   curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install-ui.sh | sudo bash
#   sudo ./install-ui.sh [version]
#   sudo ./install-ui.sh --from-source
#   sudo ./install-ui.sh --from-source --with-webmin   # optional panel module
#
# Requires core PQPM (pqpm + pqpmd) already installed.
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

if [ "$(id -u)" -ne 0 ]; then
    error "This script must be run as root (use sudo)"
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

FROM_SOURCE=0
WITH_WEBMIN=0
VERSION=""
for arg in "$@"; do
    case "$arg" in
        --from-source) FROM_SOURCE=1 ;;
        --with-webmin) WITH_WEBMIN=1 ;;
        -*) error "Unknown flag: $arg" ;;
        *) VERSION="$arg" ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || true)"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

install_webmin_module() {
    local src="$1"
    if [ "$WITH_WEBMIN" -ne 1 ]; then
        return 0
    fi
    local webmin_dir=""
    if [ -d /usr/share/webmin ]; then
        webmin_dir=/usr/share/webmin
    elif [ -d /opt/webmin ]; then
        webmin_dir=/opt/webmin
    else
        warn "Webmin not found; skipping --with-webmin. Standalone pqpm-ui is installed."
        return 0
    fi
    info "Installing optional Webmin module to $webmin_dir/pqpm ..."
    install -d "$webmin_dir/pqpm/lang"
    install -m 0755 "$src"/index.cgi "$src"/action.cgi "$src"/config.cgi "$src"/refresh.cgi "$webmin_dir/pqpm/"
    install -m 0644 "$src"/pqpm-lib.pl "$src"/module.info "$src"/config "$src"/config.info "$webmin_dir/pqpm/"
    install -m 0644 "$src"/lang/en "$webmin_dir/pqpm/lang/en"
    if [ -d /etc/webmin ]; then
        install -d /etc/webmin/pqpm
        if [ ! -f /etc/webmin/pqpm/config ]; then
            install -m 0644 "$src"/config /etc/webmin/pqpm/config
        fi
    fi
    info "Webmin module installed. Enable it under Webmin → Un-used Modules."
}

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
