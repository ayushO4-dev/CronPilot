-- Tasks (ladder-logic automation) and their run history.

CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    data        TEXT NOT NULL,        -- JSON: full task definition (trigger, rungs, ...)
    last_run    INTEGER,
    last_status TEXT,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE task_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    ts          INTEGER NOT NULL,
    trigger     TEXT NOT NULL,        -- manual | schedule
    ok          INTEGER NOT NULL,
    summary     TEXT,
    detail      TEXT,                 -- JSON: per-action results
    duration_ms INTEGER NOT NULL
);
CREATE INDEX idx_task_runs_task ON task_runs(task_id, id DESC);
