#!/usr/bin/env bash
set -euo pipefail

# PQPM Installer
# Usage:
#   curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install.sh | sudo bash
#   sudo ./install.sh [version]
#   sudo ./install.sh --from-source          # build from this git checkout
#   sudo ./install.sh --from-source v0.2.0   # build from source, stamp version

REPO="pqpm/pqpm"
INSTALL_DIR="/usr/local/bin"

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

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    *) error "Unsupported architecture: $ARCH" ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
    error "PQPM only supports Linux (detected: $OS)"
fi

FROM_SOURCE=0
VERSION=""
for arg in "$@"; do
    case "$arg" in
        --from-source) FROM_SOURCE=1 ;;
        -*) error "Unknown flag: $arg" ;;
        *) VERSION="$arg" ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || true)"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

install_runtime_dirs() {
    install -d -m 0755 /var/run/pqpm
    install -d -m 0755 /var/log/pqpm
    install -d -m 0755 /var/log/pqpm/users
    install -d -m 0755 /var/lib/pqpm
}

install_systemd_unit() {
    local unit_src="${1:-}"
    if ! command -v systemctl >/dev/null 2>&1; then
        warn "systemd not found; skip unit install. Start pqpmd manually."
        return 0
    fi
    info "Installing systemd service..."
    if [ -n "$unit_src" ] && [ -f "$unit_src" ]; then
        install -m 0644 "$unit_src" /etc/systemd/system/pqpmd.service
    else
        cat > /etc/systemd/system/pqpmd.service << 'UNIT'
[Unit]
Description=PQPM Process Manager Daemon
Documentation=https://github.com/pqpm/pqpm
After=network.target local-fs.target

[Service]
Type=simple
ExecStartPre=/usr/bin/sleep 2
ExecStart=/usr/local/bin/pqpmd
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
Delegate=yes
StandardOutput=journal
StandardError=journal
SyslogIdentifier=pqpmd
ProtectSystem=false
ProtectHome=false
PrivateTmp=false
NoNewPrivileges=false

[Install]
WantedBy=multi-user.target
UNIT
    fi
    systemctl daemon-reload
    info "Systemd service installed. Enable with: systemctl enable --now pqpmd"
}

USE_SOURCE=0
if [ "$FROM_SOURCE" -eq 1 ]; then
    USE_SOURCE=1
elif [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/cmd/daemon/main.go" ] && [ -f "$SCRIPT_DIR/cmd/cli/main.go" ]; then
    USE_SOURCE=1
fi

if [ "$USE_SOURCE" -eq 1 ]; then
    ROOT="${SCRIPT_DIR:-.}"
    if [ ! -f "$ROOT/cmd/daemon/main.go" ] || [ ! -f "$ROOT/cmd/cli/main.go" ]; then
        error "Source tree not found. Clone the repo and run: sudo ./install.sh --from-source"
    fi
    info "Building PQPM from source..."
    command -v go >/dev/null 2>&1 || error "Go toolchain required for --from-source"
    VERSION_FLAG="${VERSION:-dev}"
    COMMIT="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
    DATE="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    LDFLAGS="-s -w -X github.com/pqpm/pqpm/internal/version.Version=${VERSION_FLAG} -X github.com/pqpm/pqpm/internal/version.Commit=${COMMIT} -X github.com/pqpm/pqpm/internal/version.Date=${DATE}"
    (
        cd "$ROOT"
        go build -trimpath -ldflags "$LDFLAGS" -o "$TMPDIR/pqpmd" ./cmd/daemon
        go build -trimpath -ldflags "$LDFLAGS" -o "$TMPDIR/pqpm" ./cmd/cli
    )
    info "Installing to $INSTALL_DIR..."
    install -m 0755 "$TMPDIR/pqpmd" "$INSTALL_DIR/pqpmd"
    install -m 0755 "$TMPDIR/pqpm"  "$INSTALL_DIR/pqpm"
    install_runtime_dirs
    install_systemd_unit "$ROOT/init/pqpmd.service"
    DISPLAY_VERSION="$VERSION_FLAG"
else
    if [ -z "$VERSION" ]; then
        info "Fetching latest release..."
        VERSION=$(curl -sSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | head -n1 | cut -d'"' -f4)
        if [ -z "$VERSION" ]; then
            error "Failed to determine latest version"
        fi
    fi

    info "Installing PQPM $VERSION ($OS/$GOARCH)..."
    TARBALL="pqpm-${VERSION}-${OS}-${GOARCH}.tar.gz"
    URL="https://github.com/$REPO/releases/download/${VERSION}/${TARBALL}"

    info "Downloading $URL..."
    curl -fsSL -o "$TMPDIR/$TARBALL" "$URL" || error "Download failed. Check the version exists, or use: sudo ./install.sh --from-source"

    info "Extracting..."
    tar xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"

    # Release archives may use bare names or *-linux-arch names.
    if [ -f "$TMPDIR/pqpmd" ] && [ -f "$TMPDIR/pqpm" ]; then
        :
    elif [ -f "$TMPDIR/pqpmd-${OS}-${GOARCH}" ] && [ -f "$TMPDIR/pqpm-${OS}-${GOARCH}" ]; then
        mv "$TMPDIR/pqpmd-${OS}-${GOARCH}" "$TMPDIR/pqpmd"
        mv "$TMPDIR/pqpm-${OS}-${GOARCH}" "$TMPDIR/pqpm"
    else
        error "Archive missing pqpmd/pqpm binaries"
    fi

    info "Installing to $INSTALL_DIR..."
    install -m 0755 "$TMPDIR/pqpmd" "$INSTALL_DIR/pqpmd"
    install -m 0755 "$TMPDIR/pqpm"  "$INSTALL_DIR/pqpm"
    install_runtime_dirs
    install_systemd_unit ""
    DISPLAY_VERSION="$VERSION"
fi

info "-------------------------------------------"
info "PQPM $DISPLAY_VERSION installed successfully!"
info ""
info "  Daemon:  $INSTALL_DIR/pqpmd"
info "  CLI:     $INSTALL_DIR/pqpm"
info ""
info "Quick start:"
info "  1. Start the daemon:  sudo systemctl enable --now pqpmd"
info "  2. Create config:     cp example.pqpm.toml ~/.pqpm.toml"
info "  3. Edit your config:  nano ~/.pqpm.toml"
info "  4. Start a service:   pqpm start my-worker"
info "  5. Check status:      pqpm status"
info "-------------------------------------------"
