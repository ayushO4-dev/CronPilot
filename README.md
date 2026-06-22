# 🚀 CronPilot

**CronPilot** is a lightweight, self-hosted Linux server management suite delivered as a single Go binary. It provides a secure, unified dashboard to monitor system resources, manage `systemd` services, interact with a web terminal, and execute complex automation via a unique ladder-logic engine.

![Version](https://img.shields.io/badge/version-0.2.1-orange)
![License](https://img.shields.io/badge/License-MIT-blue)
![Go](https://img.shields.io/badge/Language-Go-00ADD8?logo=go)
![React](https://img.shields.io/badge/Frontend-React%20%2B%20TypeScript-61DAFB?logo=react)

---

## ✨ Key Features

- 🖥️ **Unified Dashboard:** Live system resource monitoring and a high-level overview of your server's health.
- ⚙️ **Service Management:** Full `systemd` control (start, stop, restart, enable/disable) and journal log viewing.
- 💻 **Web Terminal:** A full-featured, low-latency web terminal using Xterm.js and local PTY shells.
- ⚡ **Process Monitor:** Real-time view of running applications with per-process details and signal handling (TERM/KILL/HUP/INT).
- 🪜 **Ladder-Logic Engine:** A powerful automation system where tasks are defined as "rungs" of logic gates, allowing for complex conditional execution based on metrics, files, or states.
- 🔄 **Self-Updating:** Check for and install new releases right from the dashboard — downloaded server-side and **SHA-256 verified** — plus a one-line `curl | sh` installer.
- 🔐 **Hardened Security:**
    - Argon2id password hashing.
    - Optional TOTP 2FA (RFC 6238).
    - HttpOnly + SameSite session cookies.
    - Scoped `sudoers` permissions (minimal privilege principle).
    - Built-in TLS support.

---

## 🏗️ Architecture & Design

CronPilot is built with a "Security First" philosophy, inspired by tools like Cockpit:

- **Single Binary:** The Go daemon embeds the React frontend (`web/dist`) via `embed.FS`, making deployment as simple as moving one file.
- **Local Agent Model:** It manages the host it runs on directly; metrics are gathered locally, and the terminal is a local PTY shell.
- **Least Privilege:** Runs as a dedicated unprivileged service user. Privileged actions (like `systemctl`) are performed via an allowlisted `sudoers` file rather than running the daemon as root.

### Tech Stack
- **Backend:** Go (`net/http`, `gorilla/websocket`, `creack/pty`, `gopsutil/v4`, SQLite, Argon2id).
- **Frontend:** React, TypeScript, Vite, `@xterm/xterm`, `uPlot`.

---

## 🚀 Getting Started

### Install on Linux (one line)

```bash
curl -LsSf https://raw.githubusercontent.com/ayushO4-dev/CronPilot/main/deploy/bootstrap.sh | sudo sh
```

This downloads the latest release for your architecture (**amd64 / arm64 / armv7**),
verifies its checksum, and sets up a dedicated service user, an allowlisted
`sudoers` file, a self-signed TLS cert, and a `systemd` unit serving **HTTPS** on
port `8765`. Grab the generated admin password from the log:

```bash
journalctl -u cronpilot -b | grep -A2 'initial admin'
```

Then open `https://<host>:8765` (accept the self-signed cert once) and change the
password. Update anytime from **Settings → Software update**, or `sudo ./deploy/update.sh`.
Full guide: **[docs/deploy.md](docs/deploy.md)**.

> Requires a published GitHub release. Until then, build from source (below).

### Prerequisites
- **Go 1.23+** and **Node 18+**.
- A **Linux** host (requires systemd, `/proc`, and PTYs).
- *Note: For Windows development, use WSL2 with systemd enabled.*

### Build from source
To build and verify on your target machine:

```powershell
# 1. Build the frontend (Windows/Host side)
cd web; npm install; npm run build; cd ..

# 2. Build and Run the daemon (Linux/WSL side)
cd /path/to/CronPilot
go mod tidy
make build
./bin/cronpilotd # Use -dev for non-TLS local testing
```
*On first run, the daemon will print a generated **admin** password. Access the UI at `http://127.0.0.1:8765`.*

### Development Loop (Hot Reload)
For active development using WSL2 and Windows:

**Backend (WSL):**
```bash
make dev
```
**Frontend (Windows):**
```powershell
cd web; npm run dev # Opens http://localhost:5173
```

---

## ⚙️ Configuration

| Environment Variable | Flag | Default | Description |
| :--- | :--- | :--- | :--- |
| `CRONPILOT_ADDR` | `-addr` | `127.0.0.1:8765` | Listen address |
| `CRONPILOT_DATA_DIR` | `-data-dir` | `~/.local/state/cronpilot` | DB and state storage path |
| `CRONPILOT_DEV` | `-dev` | `false` | Relax secure cookies & CORS for development |
| `CRONPILOT_SHELL` | `-shell` | `user shell` | Default shell for the web terminal |
| `CRONPILOT_TLS_CERT` | `-tls-cert` | — | Path to TLS certificate |
| `CRONPILOT_TLS_KEY` | `-tls-key` | — | Path to TLS private key |
| `CRONPILOT_UPDATE_REPO` | — | `ayushO4-dev/CronPilot` | GitHub repo for self-update checks |

---

## 🗺️ Roadmap & Progress

- **Phase 1: Core Infrastructure (Done)** - Auth system, DB migrations, and base API.
- **Phase 2: Services (Done)** - `systemctl` integration, journal logs, and service state management.
- **Phase 3: Applications (Done)** - Process monitoring with CPU deltas and signal handling.
- **Phase 4: Tasks (Done)** - Ladder-logic engine implementation and UI editor. See [docs/task-editor.md](docs/task-editor.md) for examples.
- **Phase 5: Hardening & Packaging (In Progress)**
    - ✅ TOTP 2FA Implementation.
    - ✅ Built-in TLS Support (HTTPS by default).
    - ✅ Production Packaging (systemd unit, scoped sudoers, `curl | sh` installer).
    - ✅ Self-update from GitHub Releases (checksum-verified) + CI release builds (amd64/arm64/armv7).
    - ⏳ **RBAC** (Role-Based Access Control) - *Next Milestone.*

---

## 📂 Project Layout

```text
cmd/cronpilotd       # Daemon entrypoint
internal/config      # Configuration management
internal/store       # SQLite + migrations (users, sessions, settings, audit)
internal/auth        # Argon2id, sessions, rate limiting
internal/system      # gopsutil metrics (snapshot + live sampler)
internal/terminal    # PTY shell sessions
internal/services    # systemd units (systemctl/journalctl)
internal/processes   # process list & signals (gopsutil)
internal/tasks       # ladder-logic engine (model, scheduler, executor)
internal/server      # router, middleware, handlers, embedded SPA
web/                 # React + TypeScript frontend (Vite)
```

---

## 🛡️ Security Notes
- **Reverse Proxy:** It is recommended to bind to loopback and use a TLS reverse proxy. Alternatively, use the built-in `-tls-cert` flags.
- **Sudoers:** Never grant blanket root access. Only `systemctl` and `kill` are permitted via [`deploy/cronpilot.sudoers`](deploy/cronpilot.sudoers).
- **Documentation:** For a full production deployment guide, see [docs/deploy.md](docs/deploy.md).
