#!/usr/bin/env bash
# CronPilot uninstaller — run as root on the host where it was installed.
#
#   sudo ./deploy/uninstall.sh           # remove service, binary, sudoers (keeps data + user)
#   sudo ./deploy/uninstall.sh --purge   # ALSO delete /var/lib/cronpilot and the cronpilot user
#
# Mirrors deploy/install.sh. Safe to re-run; missing components are skipped.
set -uo pipefail

BIN_DST="/opt/cronpilot/bin/cronpilotd"
BIN_LINK="/usr/local/bin/cronpilotd"
SERVICE_USER="cronpilot"
UNIT_DST="/etc/systemd/system/cronpilot.service"
SUDOERS_DST="/etc/sudoers.d/cronpilot"
STATE_DIR="/var/lib/cronpilot"

PURGE=0
for arg in "$@"; do
  case "$arg" in
    --purge) PURGE=1 ;;
    -h|--help) sed -n '2,6p' "$0"; exit 0 ;;
    *) echo "error: unknown option '$arg'" >&2; exit 2 ;;
  esac
done

if [[ $EUID -ne 0 ]]; then
  echo "error: must run as root (try: sudo $0)" >&2
  exit 1
fi

echo "==> stopping + disabling cronpilot.service"
systemctl disable --now cronpilot.service 2>/dev/null || true

echo "==> removing systemd unit"
rm -f "$UNIT_DST"
systemctl daemon-reload
systemctl reset-failed cronpilot.service 2>/dev/null || true

echo "==> removing sudoers allowlist"
rm -f "$SUDOERS_DST"

echo "==> removing binary"
rm -f "$BIN_DST" "$BIN_LINK"

if [[ $PURGE -eq 1 ]]; then
  echo "==> [purge] removing state directory $STATE_DIR"
  rm -rf "$STATE_DIR"
  echo "==> [purge] removing /opt/cronpilot and TLS/config /etc/cronpilot"
  rm -rf /opt/cronpilot /etc/cronpilot
  if id -u "$SERVICE_USER" >/dev/null 2>&1; then
    echo "==> [purge] deleting service user '$SERVICE_USER'"
    userdel "$SERVICE_USER" 2>/dev/null || true
  fi
else
  echo
  echo "Kept the database at $STATE_DIR, certs under /etc/cronpilot, and the '$SERVICE_USER' user."
  echo "Re-run with --purge to remove them too."
fi

echo
echo "CronPilot uninstalled."
