#!/usr/bin/env bash
# CronPilot installer — run as root on the target Linux host.
#
#   sudo ./deploy/install.sh [path-to-binary]
#
# Installs the cronpilotd binary, a dedicated unprivileged service user, an
# allowlisted sudoers file, and a systemd unit, then enables and starts the
# service. Build the binary first (it embeds the web UI):
#
#   make web && make build-linux      # -> bin/cronpilotd-linux-amd64
#   sudo ./deploy/install.sh bin/cronpilotd-linux-amd64
#
#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"

# Detect architecture
case "$(uname -m)" in
  x86_64|amd64)
    DEFAULT_BIN="bin/cronpilotd-linux-amd64"
    ;;
  aarch64|arm64)
    DEFAULT_BIN="bin/cronpilotd-linux-arm64"
    ;;
  *)
    echo "error: unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

BIN_SRC="${1:-$DEFAULT_BIN}"
BIN_DST="/usr/local/bin/cronpilotd"
SERVICE_USER="cronpilot"
UNIT_DST="/etc/systemd/system/cronpilot.service"
SUDOERS_DST="/etc/sudoers.d/cronpilot"

if [[ $EUID -ne 0 ]]; then
  echo "error: must run as root (try: sudo $0)" >&2
  exit 1
fi

if [[ ! -f "$BIN_SRC" ]]; then
  echo "error: binary not found at '$BIN_SRC'." >&2
  echo "Build it first for your architecture." >&2
  echo
  echo "Examples:"
  echo "  make web && make build-linux-amd64"
  echo "  make web && make build-linux-arm64"
  echo
  echo "Then run:"
  echo "  sudo $0 [path-to-binary]"
  exit 1
fi

echo "==> creating service user '$SERVICE_USER' (if missing)"
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system \
    --home-dir /var/lib/cronpilot \
    --shell /bin/bash \
    "$SERVICE_USER"
fi

echo "==> installing binary -> $BIN_DST"
install -m 0755 "$BIN_SRC" "$BIN_DST"

echo "==> installing sudoers -> $SUDOERS_DST"
install -m 0440 "$HERE/cronpilot.sudoers" "$SUDOERS_DST"
if ! visudo -cf "$SUDOERS_DST"; then
  echo "error: sudoers validation failed; removing $SUDOERS_DST" >&2
  rm -f "$SUDOERS_DST"
  exit 1
fi

echo "==> installing systemd unit -> $UNIT_DST"
install -m 0644 "$HERE/cronpilot.service" "$UNIT_DST"

if getent group systemd-journal >/dev/null 2>&1; then
  usermod -aG systemd-journal "$SERVICE_USER" || true
fi

echo "==> enabling + starting cronpilot.service"
systemctl daemon-reload
systemctl enable --now cronpilot.service

echo
echo "CronPilot is running. Retrieve the generated admin password with:"
echo "  journalctl -u cronpilot -b | grep -A2 'initial admin'"
echo "Then open http://127.0.0.1:8765 (or your reverse-proxied URL) and change it."
