#!/usr/bin/env bash
# Install or upgrade Config Manager from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/msutara/config-manager-core/main/scripts/install.sh | sudo bash
#   sudo bash install.sh
#   sudo bash install.sh --version 0.2.0
#   sudo bash install.sh --arch armhf
set -euo pipefail

REPO="msutara/config-manager-core"
VERSION=""
ARCH=""

usage() {
    cat <<EOF
Usage: sudo bash install.sh [OPTIONS]

Options:
  --version VERSION   Install a specific version (e.g., 0.2.0). Default: latest.
  --arch ARCH         Override architecture detection (armhf, arm64, amd64).
  -h, --help          Show this help message.

Examples:
  curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install.sh | sudo bash
  sudo bash install.sh --version 0.2.0
  sudo bash install.sh --arch armhf
EOF
    exit 0
}

die() { echo "Error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

# Parse arguments.
while [ $# -gt 0 ]; do
    case "$1" in
        --version)
            [ $# -ge 2 ] || die "--version requires a value"
            VERSION="$2"; shift 2 ;;
        --arch)
            [ $# -ge 2 ] || die "--arch requires a value"
            ARCH="$2"; shift 2 ;;
        -h|--help) usage ;;
        *)         die "Unknown option: $1" ;;
    esac
done

# Require root.
if [ "$(id -u)" -ne 0 ]; then
    die "This script must be run as root (use sudo)."
fi

# Detect architecture.
if [ -z "$ARCH" ]; then
    if ! command -v dpkg >/dev/null 2>&1; then
        die "Cannot detect architecture: dpkg not found. Use --arch to specify."
    fi
    ARCH=$(dpkg --print-architecture)
fi

case "$ARCH" in
    armhf|arm64|amd64) ;;
    *) die "Unsupported architecture: $ARCH. Supported: armhf, arm64, amd64." ;;
esac

# Pick download tool.
if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL -o "$1" "$2"; }
    fetch_json() { curl -fsSL -H "Accept: application/vnd.github+json" "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO "$1" "$2"; }
    fetch_json() { wget -qO- --header="Accept: application/vnd.github+json" "$1"; }
else
    die "Neither curl nor wget found. Install one and retry."
fi

# Resolve version.
if [ -z "$VERSION" ]; then
    info "Fetching latest release..."
    RAW=$(fetch_json "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null) || true
    [ -z "$RAW" ] && die "Could not fetch release info. Check network or use --version."
    TAG=$(echo "$RAW" | grep -m 1 '"tag_name"' | sed 's/.*"tag_name": *"v\?\([^"]*\)".*/\1/') || true
    [ -z "$TAG" ] && die "Could not determine latest version. Use --version to specify."
    VERSION="$TAG"
fi

# Strip leading v and validate format.
VERSION="${VERSION#v}"
if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+([-.][a-zA-Z0-9]+)*$'; then
    die "Invalid version format: $VERSION"
fi

info "Installing cm v$VERSION for $ARCH..."

# Create secure temp directory.
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

DEB_FILE="$WORK_DIR/cm_${VERSION}_${ARCH}.deb"
URL="https://github.com/$REPO/releases/download/v${VERSION}/cm_${VERSION}_${ARCH}.deb"

info "Downloading $URL"
fetch "$DEB_FILE" "$URL" || die "Failed to download package from $URL. Check that version $VERSION exists and that your network is working."

# Install.
info "Installing package..."
if ! dpkg -i "$DEB_FILE"; then
    info "dpkg failed (likely missing dependencies). Attempting to fix..."
    apt-get update -qq || true
    apt-get install -f -y || die "Installation failed and dependencies could not be fixed."
fi

# Verify.
if command -v cm >/dev/null 2>&1; then
    INSTALLED=$(cm --version) || true
    info "Installed: ${INSTALLED:-unknown}"
else
    die "cm not found in PATH after install."
fi

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    if systemctl is-active --quiet cm 2>/dev/null; then
        if systemctl try-restart cm 2>/dev/null; then
            info "Service cm restarted."
        else
            info "Warning: service cm failed to restart. Check: sudo systemctl status cm"
        fi
    elif systemctl is-enabled --quiet cm 2>/dev/null; then
        info "Service cm is enabled but not running. Start with: sudo systemctl start cm"
    fi
fi

info "Done."