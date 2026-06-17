# Deploying CronPilot

CronPilot ships as a **single binary** that embeds the web UI. A production
install runs it as a dedicated unprivileged user under systemd, grants a small
allowlist of privileged commands via `sudo`, and serves HTTPS (built-in or via a
reverse proxy). The [`deploy/`](../deploy) directory has everything needed.

## 1. Build the binary

The binary embeds `web/dist`, so build the frontend first. On the target host
(or any Linux box / WSL2 with Go 1.23+ and Node 18+):

```bash
make web           # build the React frontend into web/dist
make build-linux   # cgo-free static binary -> bin/cronpilotd-linux-amd64
```

`build-linux` sets `CGO_ENABLED=0`, so the result is portable across Linux
distributions (the SQLite driver is pure Go).

## 2. Install

Copy the repo (or just `bin/cronpilotd-linux-amd64` plus the `deploy/` folder)
to the server and run the installer as root:

```bash
sudo ./deploy/install.sh bin/cronpilotd-linux-amd64
```

It will:

1. create the system user **`cronpilot`** (if missing),
2. install the binary to `/usr/local/bin/cronpilotd`,
3. install and **validate** the sudoers allowlist at `/etc/sudoers.d/cronpilot`,
4. generate a **self-signed TLS certificate** at `/etc/cronpilot/tls/` (HTTPS is
   on by default; an existing cert there is kept, not overwritten),
5. install the unit at `/etc/systemd/system/cronpilot.service`,
6. add `cronpilot` to the `systemd-journal` group (for reading service logs),
7. `daemon-reload`, then `enable --now` the service.

### First login

On first start the daemon creates an `admin` account with a random password and
writes it to the journal:

```bash
journalctl -u cronpilot -b | grep -A2 'initial admin'
```

Open `https://<host>:8765` and accept the self-signed certificate warning once
(the cert covers `localhost`, the hostname, and the host's LAN IPs). Sign in,
change the password when prompted, then enable **two-factor authentication** in
**Settings → Two-factor authentication** (see §5).

To set the initial password yourself instead, uncomment `CRONPILOT_ADMIN_USER`
/ `CRONPILOT_ADMIN_PASSWORD` in the unit before first start.

## 3. TLS

The installer turns on **built-in HTTPS by default** with a generated
self-signed cert — so a fresh install is reachable over `https://` immediately,
and the `Secure` session cookie works (browsers drop it over plain HTTP, which
is the usual cause of "login does nothing"). The unit ships with:

```ini
Environment=CRONPILOT_ADDR=0.0.0.0:8765
Environment=CRONPILOT_TLS_CERT=/etc/cronpilot/tls/cert.pem
Environment=CRONPILOT_TLS_KEY=/etc/cronpilot/tls/key.pem
```

**Use your own / a CA-signed cert:** drop the PEM files at those two paths
(`chown cronpilot:cronpilot`, key mode `0600`) and `systemctl restart cronpilot`.

**Prefer a reverse proxy?** (handles certs/renewal). Comment the two `TLS` lines
out, set `CRONPILOT_ADDR=127.0.0.1:8765`, and terminate TLS in front of it.

Caddy:

```
manage.example.com {
    reverse_proxy 127.0.0.1:8765
}
```

nginx (note the WebSocket upgrade headers — the terminal and live monitor need
them):

```nginx
server {
    listen 443 ssl;
    server_name manage.example.com;
    ssl_certificate     /etc/letsencrypt/live/manage.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/manage.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8765;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        "upgrade";
    }
}
```

## 4. The privilege model

The daemon process is unprivileged. It escalates **only** these commands through
non-interactive `sudo -n`, defined in [`deploy/cronpilot.sudoers`](../deploy/cronpilot.sudoers):

- `systemctl start|stop|restart|enable|disable <unit>` — service management.
- `kill <signal> <pid>` — sending signals from the Applications view.

Everything else (metrics, process listing, journal reads) runs as `cronpilot`
with no elevation. If the sudoers file is absent, those actions simply fail with
*"sudo: a password is required"* and are recorded as errors — the rest of the UI
keeps working.

Two privileges are **opt-in** and not granted by default:

- **Task "Run as"** — running task commands as another account needs an explicit
  `cronpilot ALL=(thatuser) NOPASSWD: ...` rule (commented in the sudoers file).
- **Tighter scoping** — the defaults allow systemctl on any unit; you can
  replace the wildcards with explicit per-unit commands.

## 5. Two-factor authentication

CronPilot supports TOTP 2FA (RFC 6238 — Google Authenticator, Aegis, 1Password,
etc.), per user:

1. **Settings → Two-factor authentication → Set up 2FA.**
2. Scan the QR code (or enter the secret manually) in your authenticator app.
3. Enter the current 6-digit code to confirm and enable it.

After that, login requires the code in addition to the password. Disabling 2FA
requires re-entering the current password. Secrets are stored in the local
database; back it up accordingly.

## 6. Upgrades

Rebuild, replace the binary, restart:

```bash
make web && make build-linux
sudo install -m 0755 bin/cronpilotd-linux-amd64 /usr/local/bin/cronpilotd
sudo systemctl restart cronpilot
```

Database migrations run automatically at startup. If the sudoers or unit files
changed in a release, re-copy them (`sudo ./deploy/install.sh ...` is idempotent).

## 7. Uninstall

Use the uninstaller (mirrors `install.sh`). By default it removes the service,
binary, and sudoers file but **keeps your data and the service user**:

```bash
sudo ./deploy/uninstall.sh           # remove CronPilot, keep /var/lib/cronpilot + user
sudo ./deploy/uninstall.sh --purge   # also delete the database and the cronpilot user
```

If you configured built-in TLS, certs you placed under `/etc/cronpilot` are left
untouched — remove them manually if no longer needed.

## Troubleshooting

| Symptom | Fix |
|---|---|
| Service won't start | `journalctl -u cronpilot -b` — check the data dir is writable and the address is free. |
| Service actions fail with *"sudo: a password is required"* | The sudoers file is missing or invalid; re-run the installer and check `visudo -cf /etc/sudoers.d/cronpilot`. |
| Empty service logs in the UI | `cronpilot` isn't in `systemd-journal`; `sudo usermod -aG systemd-journal cronpilot && sudo systemctl restart cronpilot`. |
| Terminal / live graphs don't connect behind a proxy | The proxy isn't forwarding WebSocket upgrade headers (see the nginx example in §3). |
| Locked out (lost 2FA device) | Clear it in the DB: `sqlite3 /var/lib/cronpilot/cronpilot.db "UPDATE users SET totp_enabled=0, totp_secret='' WHERE username='admin';"` then restart. |
