# CronPilot Task Editor

CronPilot tasks are small **ladder-logic programs** — the automation style used
by industrial PLCs, mapped onto server administration. This guide explains the
model, every building block, and walks through worked examples.

## The model in 30 seconds

```
Task
 ├─ Trigger        when the task is evaluated (manual / interval / cron)
 └─ Rungs (1..n)   evaluated top to bottom on every run
     ├─ Conditions ("contacts")   ALL (AND) or ANY (OR), each can be negated
     └─ Actions    ("coils")      run only if the rung's condition is true
```

On every evaluation (a scheduler tick or **Run now**), each rung's conditions
are checked; for every rung whose condition holds ("the rung is energized"),
its actions execute in order. A rung with **no conditions is always energized**
— that's how you write a plain scheduled job.

Everything a run did (which rungs fired, every action's output/exit status,
duration) is recorded in the task's **run history**.

## Using the editor

- **Tasks** tab → **+ New** creates a task and opens it in edit mode.
  (Pressing **Cancel** before the first save discards the new task again.)
- The header holds the task name; **Save** validates and persists.
- **Run now** evaluates the task immediately — use it to test rungs safely
  before enabling a schedule.
- A task does nothing on its own until **Enabled** (the schedule only runs for
  enabled tasks; *Run now* works regardless).
- Each task shows its **ID** in a box beside the name; other tasks reference it
  in `taskState` conditions and `taskToggle` actions (the editor shows a
  dropdown of task names and stores the ID).

### Trigger

| Type | Meaning |
|---|---|
| `manual` | never scheduled — runs only via **Run now** (or another task's action) |
| `interval` | every N seconds |
| `cron` | standard 5-field cron expression — the **ⓘ** box next to the input shows a plain-English translation on hover (e.g. `0 9 * * 1-5` → "every weekday at 9:00 AM") |

**Run as** (optional): the user account task commands run as. Only effective
when the daemon runs as root (it uses `sudo -n -u <user>`); leave empty to run
as the daemon's account.

### Conditions ("contacts")

Each condition can be inverted with the **not** toggle (¬). A rung combines its
conditions with **ALL (AND)** or **ANY (OR)**.

| Kind | True when… | Parameters |
|---|---|---|
| `service` | a systemd unit is in the given state | unit (`nginx.service`), state: `active` / `inactive` / `failed` |
| `process` | a process with this exact name is running | name (`nginx`) |
| `time` | current time is inside a window | start/end `HH:MM` (overnight windows like `22:00–06:00` work) |
| `metric` | a system metric compares true | metric: `cpu` `mem` `swap` `load1` `disk`, operator, value (percent; `load1` is the raw load average; `disk` takes a mount point) |
| `file` | a path exists | path |
| `flag` | a named virtual flag is set | name |
| `taskState` | another task is enabled/disabled | task, state |

**Flags** are in-memory booleans, set by `flag` actions and read by `flag`
conditions — the glue for chaining tasks (see example 4). They reset to false
when the daemon restarts.

### Actions ("coils")

| Kind | Effect | Parameters |
|---|---|---|
| `command` | run a shell command (`sh -c`), capture output | command, timeout seconds (default 30) |
| `service` | systemd operation on a unit | action: start/stop/restart/enable/disable, unit |
| `flag` | set or clear a virtual flag | name, value |
| `taskToggle` | enable or disable another task | task, enabled |
| `log` | write a line to the daemon log (and run history) | message |

A run is marked **error** if any action fails (non-zero exit, timeout, denied
service operation); the rest of the actions still execute, and the failure
detail is in the run history.

## Examples

### 1. Plain scheduled job (cron-style)

Nightly backup at 02:30.

- **Trigger:** cron `30 2 * * *`
- **Rung 1** — conditions: *(none — always runs)*
  - action `command`: `tar czf /var/backups/etc-$(date +%F).tar.gz /etc`, timeout `300`

### 2. Watchdog — restart a service if it dies

Check every 60 seconds; if nginx is not active, restart it and leave a note.

- **Trigger:** interval `60`
- **Rung 1** — match **ALL**, conditions:
  - `service` unit `nginx.service` state `active` — **not** ✓ (true when *not* active)
  - actions:
    - `service`: `restart` `nginx.service`
    - `log`: `nginx was down — restarted`

The same pattern with a `process` condition works for non-systemd daemons.

### 3. Disk guard — clean up when usage crosses a threshold

Every 10 minutes, if the root filesystem is over 90 %, clear caches and warn.

- **Trigger:** interval `600`
- **Rung 1** — match **ALL**, conditions:
  - `metric`: `disk` (mount `/`) `>` `90`
  - actions:
    - `command`: `journalctl --vacuum-size=200M`, timeout `120`
    - `log`: `disk >90% — vacuumed journals`

### 4. Before/after chaining with flags

Task B must run only after task A has succeeded today.

**Task A — "prepare data"** (cron `0 1 * * *`):
- Rung 1 — no conditions:
  - `command`: `/opt/etl/prepare.sh`, timeout `1800`
  - `flag`: set `prepared` = true

**Task B — "publish report"** (interval `300`):
- Rung 1 — match **ALL**:
  - `flag` `prepared`
  - actions:
    - `command`: `/opt/etl/publish.sh`
    - `flag`: set `prepared` = **false** (consume the flag so B runs once per A)

### 5. Quiet hours — only act inside a time window

Restart a memory-hungry app, but only at night on weekdays' off-hours, and only
if memory is actually high.

- **Trigger:** cron `*/15 * * * *`
- **Rung 1** — match **ALL**, conditions:
  - `time` `01:00`–`05:00`
  - `metric` `mem` `>` `85`
  - actions: `service` `restart` `myapp.service`

### 6. Multiple rungs — escalation ladder

One task, two thresholds: warn at 80 % CPU, act at 95 %.

- **Trigger:** interval `120`
- **Rung 1** — `metric` `cpu` `>` `80` → `log`: `cpu above 80%`
- **Rung 2** — `metric` `cpu` `>` `95` → `command`: `systemctl restart heavy-worker.service`

Both rungs are evaluated every run; at 97 % CPU both fire, at 85 % only the
first does.

### 7. Mutual exclusion with taskState / taskToggle

Pause a noisy maintenance task while a deploy task is enabled.

- **Trigger:** interval `60`
- **Rung 1** — match **ALL**:
  - `taskState`: task *deploy* is `enabled`
  - actions: `taskToggle`: task *maintenance* → disable
- **Rung 2** — match **ALL**:
  - `taskState`: task *deploy* is `enabled` — **not** ✓
  - actions: `taskToggle`: task *maintenance* → enable

## Notes & gotchas

- **Run now ignores conditions?** No — it evaluates the ladder exactly like the
  scheduler does. A rung whose conditions are false won't fire on Run now either.
- **Order matters:** rungs run top to bottom (reorder with the ↑/↓ buttons);
  actions inside a rung run left to right.
- **Privileges:** `service` actions and `run as` need root (or allowlisted
  sudoers) on the daemon side; without them the action fails with
  `sudo: a password is required` and the run is marked error.
- **Command output** is captured into run history (truncated at 8 KiB).
- **Timeouts:** a whole task evaluation is capped at 5 minutes; individual
  commands at their own timeout.
- **Flags are volatile** — they don't survive a daemon restart; don't encode
  long-lived state in them.
- Commands are arbitrary code execution by design (like cron) — every execution
  is written to the audit log.
