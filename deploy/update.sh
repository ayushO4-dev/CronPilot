#!/usr/bin/env bash
# Update CronPilot to the latest GitHub release. Run as root.
#
#   sudo ./deploy/update.sh
#
# Downloads the binary for this machine's architecture from the latest release,
# verifies its SHA-256, installs it, and restarts the service. The in-app
# updater (Settings → Software update) does the same thing from the dashboard.
set -euo pipefail

REPO="${CRONPILOT_UPDATE_REPO:-ayushO4-dev/CronPilot}"
BIN_DST="${1:-/opt/cronpilot/bin/cronpilotd}"
SERVICE_USER="cronpilot"

if [[ $EUID -ne 0 ]]; then
  echo "error: must run as root (try: sudo $0)" >&2
  exit 1
fi
for tool in curl sha256sum; do
  command -v "$tool" >/dev/null 2>&1 || { echo "error: '$tool' is required" >&2; exit 1; }
done

case "$(uname -m)" in
  x86_64|amd64)        ARCH=amd64 ;;
  aarch64|arm64)       ARCH=arm64 ;;
  armv7l|armv6l|arm)   ARCH=arm ;;
  *) echo "error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac
ASSET="cronpilotd-linux-$ARCH"

echo "==> resolving latest release of $REPO"
TAG="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"[^"]+"' | head -1 \
  | sed -E 's/.*"([^"]+)"$/\1/')"
if [[ -z "${TAG:-}" ]]; then
  echo "error: could not determine latest release tag" >&2
  exit 1
fi
echo "    latest: $TAG ($ASSET)"

BASE="https://github.com/$REPO/releases/download/$TAG"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "==> downloading"
curl -fSL "$BASE/$ASSET" -o "$TMP/$ASSET"
curl -fsSL "$BASE/SHA256SUMS" -o "$TMP/SHA256SUMS"

echo "==> verifying checksum"
( cd "$TMP" && grep " $ASSET\$" SHA256SUMS | sha256sum -c - )

echo "==> installing -> $BIN_DST"
install -d -m 0755 "$(dirname "$BIN_DST")"
install -m 0755 "$TMP/$ASSET" "$BIN_DST"
chown -R "$SERVICE_USER:$SERVICE_USER" "$(dirname "$BIN_DST")" 2>/dev/null || true

if systemctl is-enabled cronpilot.service >/dev/null 2>&1; then
  echo "==> restarting cronpilot.service"
  systemctl restart cronpilot.service
fi
echo "CronPilot updated to $TAG."
