#!/usr/bin/env bash
# HorizonX installer — the `curl | bash` entrypoint.
#
#   curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | bash
#
# What it does:
#   1. Detect OS + architecture.
#   2. Fetch the latest release tag from GitHub.
#   3. Download the matching tarball + SHA256SUMS.
#   4. Verify the checksum.
#   5. Install the `horizonx` binary to /usr/local/bin (override HORIZONX_PREFIX).
#   6. Print next steps.
#
# Safety: never runs as root unless you explicitly want /usr/local/bin;
# never pipes curl straight into a shell without checksum verification.
set -euo pipefail

REPO="${HORIZONX_REPO:-zlnew/horizonx}"
PREFIX="${HORIZONX_PREFIX:-/usr/local}"
GITHUB="https://github.com/${REPO}"

log()  { printf '\033[0;36m[*]\033[0m %s\n' "$*"; }
ok()   { printf '\033[0;32m[+]\033[0m %s\n' "$*"; }
die()  { printf '\033[0;31m[x]\033[0m %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar  >/dev/null 2>&1 || die "tar is required"
command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required"

# --- Detect OS / arch ------------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="x86_64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "unsupported architecture: $ARCH" ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) die "unsupported OS: $OS" ;;
esac

# --- Resolve latest release tag -------------------------------------------
log "resolving latest release…"
LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
TAG="$(curl -fsSL "$LATEST_URL" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
[ -n "${TAG:-}" ] || die "could not resolve latest release tag"

TAG_NO_V="${TAG#v}"
log "latest release: ${TAG}"

# --- Download + verify -----------------------------------------------------
ASSET="horizonx-${OS}-${ARCH}.tar.gz"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

log "downloading ${ASSET}…"
curl -fsSL "${GITHUB}/releases/latest/download/${ASSET}" -o "$TMP/horizonx.tar.gz"
curl -fsSL "${GITHUB}/releases/latest/download/SHA256SUMS" -o "$TMP/SHA256SUMS"

EXPECTED="$(awk -v a="$ASSET" '$2 == a {print $1}' "$TMP/SHA256SUMS")"
[ -n "${EXPECTED:-}" ] || die "no checksum found for ${ASSET} in SHA256SUMS"

ACTUAL="$(sha256sum "$TMP/horizonx.tar.gz" | awk '{print $1}')"
if [ "$ACTUAL" != "$EXPECTED" ]; then
  die "checksum mismatch for ${ASSET} — aborting (expected ${EXPECTED}, got ${ACTUAL})"
fi
ok "checksum verified"

# --- Install ---------------------------------------------------------------
BIN_DIR="$PREFIX/bin"
mkdir -p "$BIN_DIR"
log "extracting to ${BIN_DIR}…"
if [ ! -w "$BIN_DIR" ]; then
  echo
  echo "  Cannot write to ${BIN_DIR} (permission denied)."
  echo
  echo "  Options:"
  echo "    1. Re-run with sudo:          curl -fsSL ${GITHUB}/main/install.sh | sudo bash"
  echo "    2. Install to your home dir:  HORIZONX_PREFIX=\$HOME/.local curl -fsSL ${GITHUB}/main/install.sh | bash"
  echo "       (then add ~/.local/bin to your PATH)"
  echo
  die "no write permission for ${BIN_DIR}"
fi
tar -xzf "$TMP/horizonx.tar.gz" -C "$BIN_DIR" horizonx
chmod +x "$BIN_DIR/horizonx"

ok "installed $( "$BIN_DIR/horizonx" version )"
echo
echo "  Next steps:"
echo "    1. Bootstrap a control plane:"
echo "       $BIN_DIR/horizonx setup"
echo "    2. Run the server:"
echo "       $BIN_DIR/horizonx server"
echo "    3. On app hosts, install the agent:"
echo "       curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash"
echo "       # then run: horizonx agent (with HORIZONX_SERVER_ID + HORIZONX_SERVER_API_TOKEN)"
echo "    4. Upgrade later:"
echo "       $BIN_DIR/horizonx upgrade"
