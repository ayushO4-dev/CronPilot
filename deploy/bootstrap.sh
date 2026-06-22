#!/bin/sh
# CronPilot one-line installer.
#
#   curl -LsSf https://raw.githubusercontent.com/ayushO4-dev/CronPilot/main/deploy/bootstrap.sh | sudo sh
#
# Downloads the latest release binary for this machine's architecture (verifying
# its SHA-256), then runs the standard installer: a dedicated service user, an
# allowlisted sudoers file, a self-signed TLS cert, and a systemd unit serving
# HTTPS. POSIX sh; needs root, curl and sha256sum.
set -eu

REPO="${CRONPILOT_REPO:-ayushO4-dev/CronPilot}"

die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "must run as root — pipe to 'sudo sh':
  curl -LsSf https://raw.githubusercontent.com/$REPO/main/deploy/bootstrap.sh | sudo sh"
[ "$(uname -s)" = Linux ] || die "CronPilot runs on Linux only (systemd, /proc, PTYs)"
for t in curl sha256sum; do
  command -v "$t" >/dev/null 2>&1 || die "'$t' is required"
done

case "$(uname -m)" in
  x86_64|amd64)      ARCH=amd64 ;;
  aarch64|arm64)     ARCH=arm64 ;;
  armv7l|armv6l|arm) ARCH=arm ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac
ASSET="cronpilotd-linux-$ARCH"

printf '==> resolving latest release of %s\n' "$REPO"
TAG=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"[^"]+"' | head -1 \
  | sed -E 's/.*"([^"]+)"$/\1/')
[ -n "${TAG:-}" ] || die "no published release found for $REPO (push a v* tag first)"
printf '==> installing %s (%s)\n' "$TAG" "$ASSET"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
REL="https://github.com/$REPO/releases/download/$TAG"
RAW="https://raw.githubusercontent.com/$REPO/$TAG/deploy"

printf '==> downloading binary + checksums\n'
curl -fSL  "$REL/$ASSET"     -o "$TMP/$ASSET"
curl -fsSL "$REL/SHA256SUMS" -o "$TMP/SHA256SUMS"
printf '==> verifying checksum\n'
( cd "$TMP" && grep " $ASSET\$" SHA256SUMS | sha256sum -c - >/dev/null ) \
  || die "checksum verification failed"

# Fetch the installer + its unit/sudoers templates at the same tag, then hand off.
for f in install.sh cronpilot.service cronpilot.sudoers; do
  curl -fsSL "$RAW/$f" -o "$TMP/$f" || die "could not fetch deploy/$f at $TAG"
done
chmod +x "$TMP/install.sh"

command -v bash >/dev/null 2>&1 || die "bash is required to run the installer"
"$TMP/install.sh" "$TMP/$ASSET"
