#!/usr/bin/env bash
# CronPilot installer — run as root on the target Linux host.
#
#   sudo ./deploy/install.sh [path-to-binary]
#
# Installs the cronpilotd binary, a dedicated unprivileged service user, an
# allowlisted sudoers file, a self-signed TLS certificate (HTTPS by default),
# and a systemd unit, then enables and starts the service. Build the binary
# first (it embeds the web UI):
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
TLS_DIR="/etc/cronpilot/tls"
TLS_CERT="$TLS_DIR/cert.pem"
TLS_KEY="$TLS_DIR/key.pem"

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

echo "==> ensuring TLS certificate (HTTPS by default)"
if [[ -f "$TLS_CERT" && -f "$TLS_KEY" ]]; then
  echo "    keeping existing cert at $TLS_CERT"
else
  if ! command -v openssl >/dev/null 2>&1; then
    echo "error: openssl not found — install it, or place your own cert at" >&2
    echo "       $TLS_CERT and $TLS_KEY, then re-run." >&2
    exit 1
  fi
  install -d -m 0750 "$TLS_DIR"
  # Cover loopback, the hostname, and every local IP so the cert validates
  # however the host is reached on the LAN.
  san="DNS:localhost,IP:127.0.0.1"
  host="$(hostname -f 2>/dev/null || hostname || echo cronpilot)"
  [[ -n "$host" ]] && san="$san,DNS:$host"
  for ip in $(hostname -I 2>/dev/null || true); do san="$san,IP:$ip"; done
  echo "    generating self-signed cert for: $san"
  openssl req -x509 -newkey rsa:2048 -nodes -days 825 \
    -keyout "$TLS_KEY" -out "$TLS_CERT" \
    -subj "/CN=${host:-cronpilot}" -addext "subjectAltName=$san" >/dev/null 2>&1
fi
chown -R "$SERVICE_USER:$SERVICE_USER" /etc/cronpilot
chmod 0750 "$TLS_DIR"; chmod 0644 "$TLS_CERT"; chmod 0600 "$TLS_KEY"

echo "==> enabling + starting cronpilot.service"
systemctl daemon-reload
systemctl enable --now cronpilot.service

echo
echo "CronPilot is running over HTTPS with a self-signed certificate."
echo "Retrieve the generated admin password:"
echo "  journalctl -u cronpilot -b | grep -A2 'initial admin'"
echo "Then open https://<this-host>:8765 and accept the certificate warning once."
echo "Replace $TLS_CERT / $TLS_KEY with a CA-signed cert any time."

echo
echo "CronPilot is running. Retrieve the generated admin password with:"
echo "  journalctl -u cronpilot -b | grep -A2 'initial admin'"
echo "Then open http://127.0.0.1:8765 (or your reverse-proxied URL) and change it."
