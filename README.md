# CronPilot

A self-hosted **Linux server manager** — a single Go binary that runs *on* the
server it manages and serves a minimal web UI. Secure login, a tabbed dashboard,
live system-resource monitoring, a web terminal, and (in later phases) systemd
service control, a process view, and a ladder-logic task-automation engine.

> **Status: Phase 4.** Implemented: secure auth, a unified live **dashboard**,
> **systemd service management**, a **running-applications** view, a web
> **terminal**, settings, and the **ladder-logic task engine**. Planned:
> hardening & packaging (see [Roadmap](#roadmap)).

## Design

- **Single binary.** The Go daemon embeds the built React frontend (`web/dist`)
  via `embed.FS`, so deployment is one file.
- **Local agent.** It manages the host it runs on: the terminal is a local PTY
  shell; the monitor reads local metrics.
- **Security first** (Cockpit-style): runs as a scoped non-root service user,
  binds to loopback by default, Argon2id passwords, HttpOnly+SameSite session
  cookies, login rate-limiting, security headers, and an audit log.

## Tech stack

- **Backend:** Go — `net/http`, `gorilla/websocket`, `creack/pty`, `gopsutil/v4`,
  pure-Go `modernc.org/sqlite`, `golang.org/x/crypto/argon2`.
- **Frontend:** React + TypeScript + Vite, `@xterm/xterm`, `uPlot`, CSS variables
  (minimal monospace theme, no rounded corners).

## Requirements

- **Go 1.23+** and **Node 18+**.
- A **Linux** host to *run* on (systemd, `/proc`, and PTYs are Linux features).
  On Windows, develop with **WSL2 (systemd enabled)** or a VM.

## Develop on Windows + WSL2

This repo targets Linux but is commonly edited on Windows. The reliable split:
build the **frontend with Windows Node**, build/run the **Go daemon in WSL2**
(`web/dist` is shared through the filesystem at `/mnt/<drive>/...`).

### One-time: install Go in WSL (no sudo)

```bash
# inside: wsl -d Debian
cd ~
curl -fsSLo go.tgz https://go.dev/dl/go1.23.6.linux-amd64.tar.gz
mkdir -p ~/.local && tar -C ~/.local -xzf go.tgz && rm go.tgz
echo 'export PATH=$HOME/.local/go/bin:$HOME/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc && go version
```

### Build & run the single binary (recommended verification)

```powershell
# 1) Windows: build the frontend
cd web; npm install; npm run build; cd ..
```
```bash
# 2) WSL: build and run the daemon
cd /mnt/f/Codes/CORE/CronPilot
go mod tidy
make build
./bin/cronpilotd            # add -dev for http (no TLS) local use
```

On first run the daemon prints a generated **admin** password (also flagged to be
changed on first login). Open <http://127.0.0.1:8765> from Windows.

### Hot-reload dev loop (optional)

```bash
# WSL: backend in dev mode (relaxed cookies, permissive CORS/WS origin)
make dev
```
```powershell
# Windows: Vite dev server with API/WS proxy to the backend
cd web; npm run dev   # open http://localhost:5173
```

## Configuration

| Env var | Flag | Default | Purpose |
|---|---|---|---|
| `CRONPILOT_ADDR` | `-addr` | `127.0.0.1:8765` | Listen address |
| `CRONPILOT_DATA_DIR` | `-data-dir` | `~/.local/state/cronpilot` | DB + state |
| `CRONPILOT_DEV` | `-dev` | `false` | Relax Secure cookies, CORS/WS origin |
| `CRONPILOT_SHELL` | `-shell` | user shell | Shell for the web terminal |
| `CRONPILOT_TLS_CERT` / `_KEY` | `-tls-cert` / `-tls-key` | – | Enable built-in HTTPS |
| `CRONPILOT_ADMIN_USER` / `_PASSWORD` | – | `admin` / generated | First-run admin bootstrap |

## Security notes

- Bind to loopback and front with a TLS reverse proxy, **or** set
  `-tls-cert/-tls-key` for built-in HTTPS. The `Secure` cookie flag is set
  whenever not in `-dev` mode.
- Run as a dedicated unprivileged user; grant privileged actions later via
  allowlisted `sudoers` (see `deploy/` in upcoming phases) — never blanket root.

## Project layout

```
cmd/cronpilotd     daemon entrypoint
internal/config    configuration
internal/store     SQLite + migrations (users, sessions, settings, audit)
internal/auth      Argon2id, sessions, rate limiting
internal/system    gopsutil metrics (snapshot + live sampler)
internal/terminal  PTY shell sessions
internal/services  systemd units (systemctl/journalctl)
internal/processes process list & signals (gopsutil)
internal/tasks     ladder-logic engine (model, scheduler, executor)
internal/server    router, middleware, handlers, embedded SPA
web/               React + TypeScript frontend (Vite)
```

## Roadmap

- **Phase 2 — Services (done):** systemd list/detail, start/stop/restart/enable/disable, recent journal logs. Reads via `systemctl --output=json`/`journalctl`; writes via `systemctl` (escalated with `sudo -n` when not root).
- **Phase 3 — Applications (done):** process list with delta CPU%, per-process detail, and signals (TERM/KILL/HUP/INT). Signals via `kill` (escalated with `sudo -n` when not root).
- **Phase 4 — Tasks (done):** ladder-logic automation. A task is a list of rungs;
  each rung is ALL/ANY of contacts (service/process/time/metric/file/flag/taskState,
  with negate) that, when true, run actions (command/service/flag/taskToggle/log).
  Scheduled via interval or cron (robfig/cron), plus run-now; full run history.
  UI: left task list + right ladder viewer/editor. (Form-based editor; a React Flow
  drag-drop canvas is an optional future enhancement.)
  **Guide with examples: [docs/task-editor.md](docs/task-editor.md).**
- **Phase 5 — Hardening:** TOTP 2FA, RBAC, built-in TLS, packaging (systemd unit,
  scoped sudoers, installer).
