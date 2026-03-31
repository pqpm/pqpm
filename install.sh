#!/usr/bin/env bash
set -euo pipefail

# PQPM Installer
# Usage: curl -sSL https://raw.githubusercontent.com/pqpm/pqpm/main/install.sh | sudo bash
# Or:    sudo ./install.sh [version]

REPO="pqpm/pqpm"
INSTALL_DIR="/usr/local/bin"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# Check root
if [ "$(id -u)" -ne 0 ]; then
    error "This script must be run as root (use sudo)"
fi

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    arm64)   GOARCH="arm64" ;;
    *)       error "Unsupported architecture: $ARCH" ;;
esac

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
    error "PQPM only supports Linux (detected: $OS)"
fi

# Determine version
VERSION="${1:-}"
if [ -z "$VERSION" ]; then
    info "Fetching latest release..."
    VERSION=$(curl -sSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
    if [ -z "$VERSION" ]; then
        error "Failed to determine latest version"
    fi
fi

info "Installing PQPM $VERSION ($OS/$GOARCH)..."

# Download
TARBALL="pqpm-${VERSION}-${OS}-${GOARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/${VERSION}/${TARBALL}"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

info "Downloading $URL..."
curl -sSL -o "$TMPDIR/$TARBALL" "$URL" || error "Download failed. Check the version exists."

# Extract
info "Extracting..."
tar xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"

# Install binaries
info "Installing to $INSTALL_DIR..."
install -m 0755 "$TMPDIR/pqpmd" "$INSTALL_DIR/pqpmd"
install -m 0755 "$TMPDIR/pqpm"  "$INSTALL_DIR/pqpm"

# Create runtime directories
install -d -m 0755 /var/run/pqpm
install -d -m 0755 /var/log/pqpm
install -d -m 0755 /var/log/pqpm/users

# Install systemd service if systemd is available
if command -v systemctl &> /dev/null; then
    info "Installing systemd service..."
    cat > /etc/systemd/system/pqpmd.service << 'UNIT'
[Unit]
Description=PQPM Process Manager Daemon
Documentation=https://github.com/pqpm/pqpm
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/pqpmd
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Security hardening
ProtectSystem=strict
ReadWritePaths=/var/run/pqpm /var/log/pqpm
ProtectHome=read-only
NoNewPrivileges=false

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    info "Systemd service installed. Enable with: systemctl enable --now pqpmd"
fi

info "-------------------------------------------"
info "PQPM $VERSION installed successfully!"
info ""
info "  Daemon:  $INSTALL_DIR/pqpmd"
info "  CLI:     $INSTALL_DIR/pqpm"
info ""
info "Quick start:"
info "  1. Start the daemon:  sudo systemctl enable --now pqpmd"
info "  2. Create config:     cp /usr/local/share/pqpm/example.pqpm.toml ~/.pqpm.toml"
info "  3. Edit your config:  nano ~/.pqpm.toml"
info "  4. Start a service:   pqpm start my-worker"
info "  5. Check status:      pqpm status"
info "-------------------------------------------"
