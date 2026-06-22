import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, isApiError } from "../../lib/api";
import type {
  Action,
  Contact,
  MatchMode,
  Rung,
  Task,
  TaskRun,
  Trigger,
  TriggerType,
} from "../../lib/types";
import { Button, Loading, StatusDot } from "../../components/ui";
import type { Status } from "../../components/ui";
import { Modal } from "../../components/Modal";
import { cronToEnglish } from "../../lib/cron";
import { useTaskEditor } from "../../lib/taskEditor";
import styles from "./Tasks.module.css";

const CONTACT_KINDS = [
  "service",
  "process",
  "time",
  "metric",
  "file",
  "flag",
  "taskState",
  "rung",
];

// Client-side rung id so freshly-added rungs can be referenced by an "if rung"
// contact before the task is saved (the backend keeps provided ids).
const rid = () => `rung_${Math.random().toString(36).slice(2, 10)}`;
const ACTION_KINDS = ["command", "service", "flag", "taskToggle", "log"];
const METRICS = ["cpu", "mem", "swap", "load1", "disk"];
const OPS = [">", ">=", "<", "<=", "==", "!="];
const SERVICE_ACTIONS = ["start", "stop", "restart", "enable", "disable"];

// --- param helpers (params decode as Record<string, unknown>) ---
const ps = (p: Record<string, unknown>, k: string, d = "") =>
  typeof p[k] === "string" ? (p[k] as string) : p[k] == null ? d : String(p[k]);
const pn = (p: Record<string, unknown>, k: string, d = 0) =>
  typeof p[k] === "number" ? (p[k] as number) : p[k] ? Number(p[k]) : d;
const pb = (p: Record<string, unknown>, k: string) => p[k] === true;

function ago(iso?: string | null): string {
  if (!iso) return "never";
  const t = new Date(iso).getTime();
  if (!t) return "never";
  const s = Math.floor((Date.now() - t) / 1000);
  if (s < 5) return "just now";
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

function rungTriggerSummary(r: Rung): string {
  const t = r.trigger;
  if (!t || t.type === "manual") return "manual";
  if (t.type === "interval") return `every ${t.intervalSeconds ?? 0}s`;
  if (t.type === "cron") return `cron ${t.cron ?? ""}`;
  return "manual";
}

// A task's schedule is the union of its rungs' triggers.
function taskTriggerSummary(t: Task): string {
  const rungs = t.rungs ?? [];
  if (rungs.length === 0) return "no rungs";
  const scheduled = rungs.filter(
    (r) => r.trigger && r.trigger.type !== "manual",
  ).length;
  const plural = rungs.length > 1 ? "s" : "";
  return scheduled > 0
    ? `${scheduled}/${rungs.length} rung${plural} scheduled`
    : `${rungs.length} rung${plural} · manual`;
}

function rungName(rungs: Rung[], id: string): string {
  const i = rungs.findIndex((r) => r.id === id);
  if (i < 0) return id ? `rung ${id}` : "rung —";
  return rungs[i].label || `rung ${i + 1}`;
}

function statusOf(t: Task): Status {
  if (t.lastStatus === "error") return "err";
  return t.enabled ? "ok" : "muted";
}

function contactLabel(c: Contact, rungs: Rung[] = []): string {
  const p = c.params;
  let s: string;
  switch (c.kind) {
    case "rung":
      s = `if ${rungName(rungs, ps(p, "rung"))}`;
      break;
    case "service":
      s = `${ps(p, "unit")} ${ps(p, "state", "active")}`;
      break;
    case "process":
      s = `proc ${ps(p, "name")}`;
      break;
    case "time":
      s = `${ps(p, "start")}–${ps(p, "end")}`;
      break;
    case "metric":
      s = `${ps(p, "metric")}${c.kind === "metric" && ps(p, "metric") === "disk" ? " " + ps(p, "mount", "/") : ""} ${ps(p, "op")} ${ps(p, "value")}`;
      break;
    case "file":
      s = `file ${ps(p, "path")}`;
      break;
    case "flag":
      s = `flag ${ps(p, "name")}`;
      break;
    case "taskState":
      s = `task ${ps(p, "task")} ${ps(p, "state", "enabled")}`;
      break;
    default:
      s = c.kind;
  }
  return (c.negate ? "¬ " : "") + s;
}

function actionLabel(a: Action): string {
  const p = a.params;
  switch (a.kind) {
    case "command":
      return `run: ${ps(p, "command")}`;
    case "service":
      return `${ps(p, "action", "restart")} ${ps(p, "unit")}`;
    case "flag":
      return `flag ${ps(p, "name")} = ${pb(p, "value")}`;
    case "taskToggle":
      return `task ${ps(p, "task")} → ${pb(p, "enabled") ? "enable" : "disable"}`;
    case "log":
      return `log: ${ps(p, "message")}`;
    default:
      return a.kind;
  }
}

function defaultContact(kind: string): Contact {
  const m: Record<string, Record<string, unknown>> = {
    service: { unit: "", state: "active" },
    process: { name: "" },
    time: { start: "09:00", end: "17:00" },
    metric: { metric: "cpu", op: ">", value: 80 },
    file: { path: "" },
    flag: { name: "" },
    taskState: { task: "", state: "enabled" },
    rung: { rung: "" },
  };
  return { kind, params: m[kind] ?? {} };
}

function defaultAction(kind: string): Action {
  const m: Record<string, Record<string, unknown>> = {
    command: { command: "" },
    service: { unit: "", action: "restart" },
    flag: { name: "", value: true },
    taskToggle: { task: "", enabled: true },
    log: { message: "" },
  };
  return { kind, params: m[kind] ?? {} };
}

export function Tasks() {
  const { selectedId, select, newTaskId, setNewTaskId } = useTaskEditor();
  const qc = useQueryClient();

  const { data: list, isLoading } = useQuery({
    queryKey: ["tasks"],
    queryFn: () => api.get<Task[]>("/api/tasks"),
    refetchInterval: 5000,
  });

  const create = useMutation({
    mutationFn: () =>
      api.post<Task>("/api/tasks", {
        name: "New task",
        enabled: false,
        rungs: [],
      }),
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: ["tasks"] });
      select(t.id);
      setNewTaskId(t.id);
    },
  });

  useEffect(() => {
    if (!selectedId && list && list.length > 0) select(list[0].id);
  }, [list, selectedId, select]);

  // Sliding selection indicator on the left of the task list.
  const listRef = useRef<HTMLDivElement>(null);
  const [indicator, setIndicator] = useState<{
    top: number;
    height: number;
  } | null>(null);
  useLayoutEffect(() => {
    const c = listRef.current;
    if (!c || !selectedId) {
      setIndicator(null);
      return;
    }
    const el = c.querySelector<HTMLElement>(`[data-task-id="${selectedId}"]`);
    setIndicator(el ? { top: el.offsetTop, height: el.offsetHeight } : null);
  }, [selectedId, list]);

  if (isLoading) return <Loading text="reading tasks" />;

  const tasks = list ?? [];

  return (
    <div className={styles.page}>
      <aside className={styles.list}>
        <div className={styles.listHead}>
          <span className={styles.listTitle}>Tasks</span>
          <Button
            small
            variant="primary"
            disabled={create.isPending}
            onClick={() => create.mutate()}
          >
            + New
          </Button>
        </div>
        <div className={styles.listBody} ref={listRef}>
          {indicator && (
            <span
              className={styles.listIndicator}
              style={{ top: indicator.top, height: indicator.height }}
            />
          )}
          {tasks.length === 0 && (
            <div className={styles.emptyList}>no tasks yet</div>
          )}
          {tasks.map((t) => (
            <button
              key={t.id}
              data-task-id={t.id}
              className={`${styles.taskItem} ${selectedId === t.id ? styles.taskItemSel : ""}`}
              onClick={() => select(t.id)}
            >
              <div className={styles.taskItemTop}>
                <StatusDot
                  status={statusOf(t)}
                  title={t.enabled ? "enabled" : "disabled"}
                />
                <span className={styles.taskName}>{t.name}</span>
              </div>
              <div className={styles.taskMeta}>
                {taskTriggerSummary(t)} · ran {ago(t.lastRun)}
              </div>
              <div className={styles.taskIdLine}>ID: {t.id}</div>
            </button>
          ))}
        </div>
      </aside>

      <section className={styles.detail}>
        {selectedId ? (
          <TaskDetail
            key={selectedId}
            id={selectedId}
            allTasks={tasks}
            isNew={selectedId === newTaskId}
            onSaved={() => setNewTaskId(null)}
            onDeleted={() => {
              select(null);
              setNewTaskId(null);
            }}
          />
        ) : (
          <div className={styles.empty}>
            Select a task, or create one with “+ New”.
          </div>
        )}
      </section>
    </div>
  );
}

function TaskDetail({
  id,
  allTasks,
  isNew,
  onSaved,
  onDeleted,
}: {
  id: string;
  allTasks: Task[];
  isNew: boolean;
  onSaved: () => void;
  onDeleted: () => void;
}) {
  const qc = useQueryClient();
  const { editMode, draft, startEdit, setDraft, stopEdit } = useTaskEditor();
  const [error, setError] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [runsLimit, setRunsLimit] = useState(20);

  const { data: task } = useQuery({
    queryKey: ["task", id],
    queryFn: () => api.get<Task>(`/api/tasks/${id}`),
  });
  const { data: runs } = useQuery({
    queryKey: ["task-runs", id, runsLimit],
    queryFn: () =>
      api.get<TaskRun[]>(`/api/tasks/${id}/runs?limit=${runsLimit}`),
    refetchInterval: 5000,
  });

  // New (empty) tasks open directly in edit mode — unless a draft for this task
  // is already in progress (e.g. preserved across a browser-tab switch).
  useEffect(() => {
    if (task && task.rungs.length === 0 && !(draft && draft.id === task.id)) {
      startEdit(structuredClone(task));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [task?.id]);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["tasks"] });
    qc.invalidateQueries({ queryKey: ["task", id] });
    qc.invalidateQueries({ queryKey: ["task-runs", id] });
  };

  const run = useMutation({
    mutationFn: () => api.post(`/api/tasks/${id}/run`),
    onSuccess: () => {
      setError("");
      invalidate();
    },
    onError: (e) => setError(isApiError(e) ? e.error : "run failed"),
  });
  const toggle = useMutation({
    mutationFn: (enabled: boolean) =>
      api.post(`/api/tasks/${id}/${enabled ? "enable" : "disable"}`),
    onSuccess: invalidate,
  });
  const save = useMutation({
    mutationFn: (t: Task) => api.put<Task>(`/api/tasks/${id}`, t),
    onSuccess: () => {
      setError("");
      stopEdit();
      onSaved();
      invalidate();
    },
    onError: (e) => setError(isApiError(e) ? e.error : "save failed"),
  });
  function doDelete() {
    fetch(`/api/tasks/${id}`, {
      method: "DELETE",
      credentials: "include",
    }).then(() => {
      setConfirmDelete(false);
      qc.invalidateQueries({ queryKey: ["tasks"] });
      onDeleted();
    });
  }

  if (!task) return <Loading text="loading task" />;

  const beginEdit = () => {
    startEdit(structuredClone(task));
    setError("");
  };
  const cancelEdit = () => {
    // A brand-new task that was never saved is discarded entirely on cancel.
    if (isNew) {
      doDelete();
      return;
    }
    stopEdit();
    setError("");
  };

  const editing = editMode && draft != null && draft.id === id;
  const view = editing && draft ? draft : task;

  return (
    <div className={styles.detailInner}>
      <header className={styles.detailHead}>
        <div className={styles.detailTitleWrap}>
          <StatusDot status={statusOf(task)} />
          {editing && draft ? (
            <input
              className={styles.titleInput}
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            />
          ) : (
            <h2 className={styles.detailTitle}>{task.name}</h2>
          )}
          <span className={styles.idBadge} title="task id">
            {task.id}
          </span>
        </div>
        <div className={styles.detailActions}>
          {!editing && (
            <>
              <Button
                small
                disabled={run.isPending}
                onClick={() => run.mutate()}
              >
                {run.isPending ? "running…" : "Run now"}
              </Button>
              <Button small onClick={() => toggle.mutate(!task.enabled)}>
                {task.enabled ? "Disable" : "Enable"}
              </Button>
              <Button small onClick={beginEdit}>
                Edit
              </Button>
              <Button
                small
                variant="danger"
                onClick={() => setConfirmDelete(true)}
              >
                Delete
              </Button>
            </>
          )}
          {editing && draft && (
            <>
              <Button
                small
                variant="primary"
                disabled={save.isPending}
                onClick={() => save.mutate(draft)}
              >
                {save.isPending ? "saving…" : "Save"}
              </Button>
              <Button small onClick={cancelEdit}>
                Cancel
              </Button>
            </>
          )}
        </div>
      </header>

      {error && (
        <div className={styles.error}>
          <span>{error}</span>
          <button className={styles.dismiss} onClick={() => setError("")}>
            ✕
          </button>
        </div>
      )}

      <div className={styles.body}>
        {editing && draft ? (
          <div className={styles.triggerEdit}>
            <span className={styles.sectionLabel}>Run as</span>
            <input
              className={styles.input}
              value={draft.runAs ?? ""}
              placeholder="user (optional; needs root/sudoers)"
              onChange={(e) => setDraft({ ...draft, runAs: e.target.value })}
            />
          </div>
        ) : (
          view.runAs && (
            <div className={styles.triggerRow}>
              <span className={styles.sectionLabel}>Run as</span>
              <span className={styles.triggerVal}>{view.runAs}</span>
            </div>
          )
        )}

        <div className={styles.sectionLabel}>Ladder</div>
        {editing && draft ? (
          <LadderEditor draft={draft} setDraft={setDraft} tasks={allTasks} />
        ) : (
          <LadderView rungs={view.rungs} />
        )}

        {!editing && (
          <>
            <div className={styles.sectionLabel}>Run history</div>
            <RunHistory
              runs={runs ?? []}
              canLoadMore={(runs?.length ?? 0) >= runsLimit}
              onLoadMore={() => setRunsLimit((n) => n + 20)}
            />
          </>
        )}
      </div>

      {confirmDelete && (
        <Modal
          title="Delete task"
          onClose={() => setConfirmDelete(false)}
          actions={
            <>
              <span style={{ flex: 1 }} />
              <Button small onClick={() => setConfirmDelete(false)}>
                Cancel
              </Button>
              <Button small variant="danger" onClick={doDelete}>
                Delete
              </Button>
            </>
          }
        >
          <p className={styles.confirmText}>
            Delete <strong>{task.name}</strong> and its run history? This cannot
            be undone.
          </p>
        </Modal>
      )}
    </div>
  );
}

// RungTriggerEdit edits a single rung's optional schedule.
function RungTriggerEdit({
  trigger,
  onChange,
}: {
  trigger?: Trigger;
  onChange: (t: Trigger | undefined) => void;
}) {
  const type = trigger?.type ?? "manual";
  const setType = (next: TriggerType) => {
    if (next === "manual") return onChange(undefined);
    if (next === "interval")
      return onChange({
        type: "interval",
        intervalSeconds: trigger?.intervalSeconds || 60,
      });
    return onChange({ type: "cron", cron: trigger?.cron || "*/5 * * * *" });
  };
  return (
    <span
        className={styles.rungTrigEdit}
        style={{ marginLeft: '0.5rem' }}
      >
      <select
        className={styles.select}
        value={type}
        onChange={(e) => setType(e.target.value as TriggerType)}
      >
        <option value="manual">manual</option>
        <option value="interval">interval</option>
        <option value="cron">cron</option>
      </select>
      {type === "interval" && (
        <input
          className={styles.numInput}
          type="number"
          min={1}
          value={trigger?.intervalSeconds ?? 60}
          title="seconds"
          onChange={(e) =>
            onChange({ type: "interval", intervalSeconds: Number(e.target.value) })
          }
        />
      )}
      {type === "cron" && (
        <span className={styles.cronField}>
          <input
            className={styles.input}
            value={trigger?.cron ?? ""}
            placeholder="*/5 * * * *"
            onChange={(e) => onChange({ type: "cron", cron: e.target.value })}
          />
          <span className={styles.cronInfo} tabIndex={0}>
            ⓘ
            <span className={styles.cronTip}>
              {cronToEnglish(trigger?.cron ?? "")}
            </span>
          </span>
        </span>
      )}
    </span>
  );
}

function LadderView({ rungs }: { rungs: Rung[] }) {
  if (rungs.length === 0)
    return (
      <div className={styles.muted}>no rungs — this task does nothing yet</div>
    );
  return (
    <div className={styles.ladder}>
      {rungs.map((r, i) => (
        <div key={r.id ?? i} className={styles.rung}>
          <span className={styles.rungTrig} title="when this rung runs">
            {rungTriggerSummary(r)}
          </span>
          <div className={styles.rungConds}>
            {r.contacts.length === 0 ? (
              <span className={`${styles.chip} ${styles.always}`}>ALWAYS</span>
            ) : (
              r.contacts.map((c, ci) => (
                <span key={c.id ?? ci} className={styles.condItem}>
                  {ci > 0 && (
                    <span className={styles.joiner}>
                      {r.match === "any" ? "OR" : "AND"}
                    </span>
                  )}
                  <span
                    className={`${styles.chip} ${c.negate ? styles.chipNeg : ""}`}
                  >
                    {contactLabel(c, rungs)}
                  </span>
                </span>
              ))
            )}
          </div>
          <div className={styles.arrow}>→</div>
          <div className={styles.rungActs}>
            {r.actions.map((a, ai) => (
              <span
                key={a.id ?? ai}
                className={`${styles.chip} ${styles.coil}`}
              >
                {actionLabel(a)}
              </span>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function RunHistory({
  runs,
  canLoadMore,
  onLoadMore,
}: {
  runs: TaskRun[];
  canLoadMore: boolean;
  onLoadMore: () => void;
}) {
  const [open, setOpen] = useState<number | null>(null);
  if (runs.length === 0) return <div className={styles.muted}>never run</div>;
  return (
    <div className={styles.runsWrap}>
      <div className={styles.runs}>
        {runs.map((r) => (
        <div key={r.id} className={styles.runRow}>
          <button
            className={styles.runHead}
            onClick={() => setOpen(open === r.id ? null : r.id)}
          >
            <StatusDot status={r.ok ? "ok" : "err"} />
            <span className={styles.runTime}>
              {new Date(r.time).toLocaleString()}
            </span>
            <span className={styles.muted}>{r.trigger}</span>
            <span className={styles.runSummary}>{r.summary}</span>
            <span className={styles.muted}>{r.durationMs}ms</span>
          </button>
          {open === r.id && r.detail && (
            <pre className={styles.runDetail}>{formatDetail(r.detail)}</pre>
          )}
        </div>
        ))}
      </div>
      {canLoadMore && (
        <button className={styles.loadMore} onClick={onLoadMore}>
          load more
        </button>
      )}
    </div>
  );
}

function formatDetail(detail: string): string {
  try {
    const items = JSON.parse(detail) as {
      kind: string;
      detail?: string;
      ok: boolean;
      output?: string;
      error?: string;
    }[];
    return items
      .map((it) => {
        const head = `[${it.ok ? "ok" : "ERR"}] ${it.kind}${it.detail ? " · " + it.detail : ""}`;
        const out = it.output ? `\n  ${it.output.replace(/\n/g, "\n  ")}` : "";
        const err = it.error ? `\n  error: ${it.error}` : "";
        return head + out + err;
      })
      .join("\n");
  } catch {
    return detail;
  }
}

// ---------- editor ----------

function LadderEditor({
  draft,
  setDraft,
  tasks,
}: {
  draft: Task;
  setDraft: (t: Task) => void;
  tasks: Task[];
}) {
  const patch = (mut: (d: Task) => void) => {
    const d = structuredClone(draft);
    mut(d);
    setDraft(d);
  };

  return (
    <div className={styles.editor}>
      {draft.rungs.map((rung, ri) => (
        <div key={rung.id ?? ri} className={styles.rungEdit}>
          <div className={styles.rungEditHead}>
            <span className={styles.rungNo}>{ri + 1}</span>
            <span className={styles.vsep} />
            <RungTriggerEdit
              trigger={rung.trigger}
              onChange={(t) =>
                patch((d) => {
                  d.rungs[ri].trigger = t;
                })
              }
            />
            <span style={{ flex: 1 }} />
            <Button
              small
              className={styles.iconBox}
              disabled={ri === 0}
              title="move up"
              onClick={() =>
                patch((d) => {
                  const r = d.rungs;
                  [r[ri - 1], r[ri]] = [r[ri], r[ri - 1]];
                })
              }
            >
              ↑
            </Button>
            <Button
              small
              className={styles.iconBox}
              disabled={ri === draft.rungs.length - 1}
              title="move down"
              onClick={() =>
                patch((d) => {
                  const r = d.rungs;
                  [r[ri + 1], r[ri]] = [r[ri], r[ri + 1]];
                })
              }
            >
              ↓
            </Button>
            <Button
              small
              variant="danger"
              onClick={() =>
                patch((d) => {
                  d.rungs.splice(ri, 1);
                })
              }
            >
              Delete
            </Button>
          </div>

          <div className={styles.editCols}>
            <div className={styles.editCol}>
              <div className={styles.sectionLabel}>Conditions</div>
              {rung.contacts.map((c, ci) => (
                <div key={c.id ?? ci} className={styles.editRow}>
                  {ci > 0 ? (
                    <select
                      className={`${styles.select} ${styles.joinSelect}`}
                      value={rung.match}
                      title="how conditions combine"
                      onChange={(e) =>
                        patch((d) => {
                          d.rungs[ri].match = e.target.value as MatchMode;
                        })
                      }
                    >
                      <option value="all">AND</option>
                      <option value="any">OR</option>
                    </select>
                  ) : (
                    <span className={styles.joinSpacer} />
                  )}
                  <button
                    type="button"
                    className={`${styles.negBtn} ${c.negate ? styles.negBtnOn : ""}`}
                    title="negate this condition (NOT)"
                    onClick={() =>
                      patch((d) => {
                        d.rungs[ri].contacts[ci].negate =
                          !d.rungs[ri].contacts[ci].negate;
                      })
                    }
                  >
                    NOT
                  </button>
                  <select
                    className={styles.select}
                    value={c.kind}
                    onChange={(e) =>
                      patch((d) => {
                        const neg = d.rungs[ri].contacts[ci].negate;
                        d.rungs[ri].contacts[ci] = defaultContact(
                          e.target.value,
                        );
                        d.rungs[ri].contacts[ci].negate = neg;
                      })
                    }
                  >
                    {CONTACT_KINDS.map((k) => (
                      <option key={k} value={k}>
                        {k}
                      </option>
                    ))}
                  </select>
                  <ContactParams
                    c={c}
                    tasks={tasks}
                    rungs={draft.rungs}
                    selfId={rung.id}
                    onParam={(k, v) =>
                      patch((d) => {
                        d.rungs[ri].contacts[ci].params[k] = v;
                      })
                    }
                  />
                  <Button
                    small
                    className={styles.iconBox}
                    title="remove condition"
                    onClick={() =>
                      patch((d) => {
                        d.rungs[ri].contacts.splice(ci, 1);
                      })
                    }
                  >
                    ✕
                  </Button>
                </div>
              ))}
              <AddSelect
                placeholder="+ condition…"
                options={CONTACT_KINDS}
                onAdd={(k) =>
                  patch((d) => {
                    d.rungs[ri].contacts.push(defaultContact(k));
                  })
                }
              />
            </div>

            <div className={styles.editCol}>
              <div className={styles.sectionLabel}>Actions</div>
              {rung.actions.map((a, ai) => (
                <div key={a.id ?? ai} className={styles.editRow}>
                  <select
                    className={styles.select}
                    value={a.kind}
                    onChange={(e) =>
                      patch((d) => {
                        d.rungs[ri].actions[ai] = defaultAction(e.target.value);
                      })
                    }
                  >
                    {ACTION_KINDS.map((k) => (
                      <option key={k} value={k}>
                        {k}
                      </option>
                    ))}
                  </select>
                  <ActionParams
                    a={a}
                    tasks={tasks}
                    onParam={(k, v) =>
                      patch((d) => {
                        d.rungs[ri].actions[ai].params[k] = v;
                      })
                    }
                  />
                  <Button
                    small
                    className={styles.iconBox}
                    title="remove action"
                    onClick={() =>
                      patch((d) => {
                        d.rungs[ri].actions.splice(ai, 1);
                      })
                    }
                  >
                    ✕
                  </Button>
                </div>
              ))}
              <AddSelect
                placeholder="+ action…"
                options={ACTION_KINDS}
                onAdd={(k) =>
                  patch((d) => {
                    d.rungs[ri].actions.push(defaultAction(k));
                  })
                }
              />
            </div>
          </div>
        </div>
      ))}
      <Button
        small
        onClick={() =>
          patch((d) => {
            d.rungs.push({
              id: rid(),
              match: "all",
              contacts: [],
              actions: [defaultAction("command")],
            });
          })
        }
      >
        + rung
      </Button>
    </div>
  );
}

function AddSelect({
  placeholder,
  options,
  onAdd,
}: {
  placeholder: string;
  options: string[];
  onAdd: (k: string) => void;
}) {
  return (
    <select
      className={styles.addSelect}
      value=""
      onChange={(e) => {
        if (e.target.value) {
          onAdd(e.target.value);
          e.target.value = "";
        }
      }}
    >
      <option value="">{placeholder}</option>
      {options.map((k) => (
        <option key={k} value={k}>
          {k}
        </option>
      ))}
    </select>
  );
}

function ContactParams({
  c,
  tasks,
  rungs,
  selfId,
  onParam,
}: {
  c: Contact;
  tasks: Task[];
  rungs: Rung[];
  selfId?: string;
  onParam: (k: string, v: unknown) => void;
}) {
  const p = c.params;
  const text = (k: string, ph: string, w?: number) => (
    <input
      className={styles.input}
      style={w ? { width: w } : undefined}
      value={ps(p, k)}
      placeholder={ph}
      onChange={(e) => onParam(k, e.target.value)}
    />
  );
  switch (c.kind) {
    case "rung":
      return (
        <select
          className={styles.select}
          value={ps(p, "rung")}
          onChange={(e) => onParam("rung", e.target.value)}
        >
          <option value="">— rung —</option>
          {rungs.map((r, i) =>
            r.id && r.id !== selfId ? (
              <option key={r.id} value={r.id}>
                {r.label || `Rung ${i + 1}`}
              </option>
            ) : null,
          )}
        </select>
      );
    case "service":
      return (
        <>
          {text("unit", "unit.service")}
          <select
            className={styles.select}
            value={ps(p, "state", "active")}
            onChange={(e) => onParam("state", e.target.value)}
          >
            <option value="active">active</option>
            <option value="inactive">inactive</option>
            <option value="failed">failed</option>
          </select>
        </>
      );
    case "process":
      return text("name", "process name");
    case "time":
      return (
        <>
          {text("start", "HH:MM", 70)}
          <span className={styles.muted}>to</span>
          {text("end", "HH:MM", 70)}
        </>
      );
    case "metric":
      return (
        <>
          <select
            className={styles.select}
            value={ps(p, "metric", "cpu")}
            onChange={(e) => onParam("metric", e.target.value)}
          >
            {METRICS.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
          <select
            className={styles.select}
            value={ps(p, "op", ">")}
            onChange={(e) => onParam("op", e.target.value)}
          >
            {OPS.map((o) => (
              <option key={o} value={o}>
                {o}
              </option>
            ))}
          </select>
          <input
            className={styles.numInput}
            type="number"
            value={pn(p, "value", 0)}
            onChange={(e) => onParam("value", Number(e.target.value))}
          />
          {ps(p, "metric") === "disk" && text("mount", "/", 90)}
        </>
      );
    case "file":
      return text("path", "/path/to/file");
    case "flag":
      return text("name", "flag name");
    case "taskState":
      return (
        <>
          <select
            className={styles.select}
            value={ps(p, "task")}
            onChange={(e) => onParam("task", e.target.value)}
          >
            <option value="">— task —</option>
            {tasks.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
          <select
            className={styles.select}
            value={ps(p, "state", "enabled")}
            onChange={(e) => onParam("state", e.target.value)}
          >
            <option value="enabled">enabled</option>
            <option value="disabled">disabled</option>
          </select>
        </>
      );
    default:
      return null;
  }
}

function ActionParams({
  a,
  tasks,
  onParam,
}: {
  a: Action;
  tasks: Task[];
  onParam: (k: string, v: unknown) => void;
}) {
  const p = a.params;
  const text = (k: string, ph: string, w?: number) => (
    <input
      className={styles.input}
      style={w ? { width: w } : undefined}
      value={ps(p, k)}
      placeholder={ph}
      onChange={(e) => onParam(k, e.target.value)}
    />
  );
  switch (a.kind) {
    case "command":
      return (
        <>
          {text("command", "shell command")}
          <input
            className={styles.numInput}
            type="number"
            value={pn(p, "timeoutSeconds", 30)}
            title="timeout (s)"
            onChange={(e) => onParam("timeoutSeconds", Number(e.target.value))}
          />
        </>
      );
    case "service":
      return (
        <>
          <select
            className={styles.select}
            value={ps(p, "action", "restart")}
            onChange={(e) => onParam("action", e.target.value)}
          >
            {SERVICE_ACTIONS.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
          {text("unit", "unit.service")}
        </>
      );
    case "flag":
      return (
        <>
          {text("name", "flag name")}
          <label className={styles.negToggle}>
            <input
              type="checkbox"
              checked={pb(p, "value")}
              onChange={(e) => onParam("value", e.target.checked)}
            />
            set
          </label>
        </>
      );
    case "taskToggle":
      return (
        <>
          <select
            className={styles.select}
            value={ps(p, "task")}
            onChange={(e) => onParam("task", e.target.value)}
          >
            <option value="">— task —</option>
            {tasks.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
          <label className={styles.negToggle}>
            <input
              type="checkbox"
              checked={pb(p, "enabled")}
              onChange={(e) => onParam("enabled", e.target.checked)}
            />
            enable
          </label>
        </>
      );
    case "log":
      return text("message", "message");
    default:
      return null;
  }
}
