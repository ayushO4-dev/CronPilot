import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, isApiError } from '../../lib/api'
import type { Action, Contact, MatchMode, Rung, Task, TaskRun, TriggerType } from '../../lib/types'
import { Button, Loading, StatusDot } from '../../components/ui'
import type { Status } from '../../components/ui'
import { Modal } from '../../components/Modal'
import styles from './Tasks.module.css'

const CONTACT_KINDS = ['service', 'process', 'time', 'metric', 'file', 'flag', 'taskState']
const ACTION_KINDS = ['command', 'service', 'flag', 'taskToggle', 'log']
const METRICS = ['cpu', 'mem', 'swap', 'load1', 'disk']
const OPS = ['>', '>=', '<', '<=', '==', '!=']
const SERVICE_ACTIONS = ['start', 'stop', 'restart', 'enable', 'disable']

// --- param helpers (params decode as Record<string, unknown>) ---
const ps = (p: Record<string, unknown>, k: string, d = '') => (typeof p[k] === 'string' ? (p[k] as string) : p[k] == null ? d : String(p[k]))
const pn = (p: Record<string, unknown>, k: string, d = 0) => (typeof p[k] === 'number' ? (p[k] as number) : p[k] ? Number(p[k]) : d)
const pb = (p: Record<string, unknown>, k: string) => p[k] === true

function ago(iso?: string | null): string {
  if (!iso) return 'never'
  const t = new Date(iso).getTime()
  if (!t) return 'never'
  const s = Math.floor((Date.now() - t) / 1000)
  if (s < 5) return 'just now'
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

function triggerSummary(t: Task['trigger']): string {
  if (t.type === 'interval') return `every ${t.intervalSeconds ?? 0}s`
  if (t.type === 'cron') return `cron ${t.cron ?? ''}`
  return 'manual'
}

function statusOf(t: Task): Status {
  if (t.lastStatus === 'error') return 'err'
  return t.enabled ? 'ok' : 'muted'
}

function contactLabel(c: Contact): string {
  const p = c.params
  let s: string
  switch (c.kind) {
    case 'service': s = `${ps(p, 'unit')} ${ps(p, 'state', 'active')}`; break
    case 'process': s = `proc ${ps(p, 'name')}`; break
    case 'time': s = `${ps(p, 'start')}–${ps(p, 'end')}`; break
    case 'metric': s = `${ps(p, 'metric')}${c.kind === 'metric' && ps(p, 'metric') === 'disk' ? ' ' + ps(p, 'mount', '/') : ''} ${ps(p, 'op')} ${ps(p, 'value')}`; break
    case 'file': s = `file ${ps(p, 'path')}`; break
    case 'flag': s = `flag ${ps(p, 'name')}`; break
    case 'taskState': s = `task ${ps(p, 'task')} ${ps(p, 'state', 'enabled')}`; break
    default: s = c.kind
  }
  return (c.negate ? '¬ ' : '') + s
}

function actionLabel(a: Action): string {
  const p = a.params
  switch (a.kind) {
    case 'command': return `run: ${ps(p, 'command')}`
    case 'service': return `${ps(p, 'action', 'restart')} ${ps(p, 'unit')}`
    case 'flag': return `flag ${ps(p, 'name')} = ${pb(p, 'value')}`
    case 'taskToggle': return `task ${ps(p, 'task')} → ${pb(p, 'enabled') ? 'enable' : 'disable'}`
    case 'log': return `log: ${ps(p, 'message')}`
    default: return a.kind
  }
}

function defaultContact(kind: string): Contact {
  const m: Record<string, Record<string, unknown>> = {
    service: { unit: '', state: 'active' },
    process: { name: '' },
    time: { start: '09:00', end: '17:00' },
    metric: { metric: 'cpu', op: '>', value: 80 },
    file: { path: '' },
    flag: { name: '' },
    taskState: { task: '', state: 'enabled' },
  }
  return { kind, params: m[kind] ?? {} }
}

function defaultAction(kind: string): Action {
  const m: Record<string, Record<string, unknown>> = {
    command: { command: '' },
    service: { unit: '', action: 'restart' },
    flag: { name: '', value: true },
    taskToggle: { task: '', enabled: true },
    log: { message: '' },
  }
  return { kind, params: m[kind] ?? {} }
}

export function Tasks() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const qc = useQueryClient()

  const { data: list, isLoading } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => api.get<Task[]>('/api/tasks'),
    refetchInterval: 5000,
  })

  const create = useMutation({
    mutationFn: () => api.post<Task>('/api/tasks', { name: 'New task', enabled: false, trigger: { type: 'manual' }, rungs: [] }),
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      setSelectedId(t.id)
    },
  })

  useEffect(() => {
    if (!selectedId && list && list.length > 0) setSelectedId(list[0].id)
  }, [list, selectedId])

  if (isLoading) return <Loading text="reading tasks" />

  const tasks = list ?? []

  return (
    <div className={styles.page}>
      <aside className={styles.list}>
        <div className={styles.listHead}>
          <span className={styles.listTitle}>Tasks</span>
          <Button small variant="primary" disabled={create.isPending} onClick={() => create.mutate()}>
            + New
          </Button>
        </div>
        <div className={styles.listBody}>
          {tasks.length === 0 && <div className={styles.emptyList}>no tasks yet</div>}
          {tasks.map((t) => (
            <button
              key={t.id}
              className={`${styles.taskItem} ${selectedId === t.id ? styles.taskItemSel : ''}`}
              onClick={() => setSelectedId(t.id)}
            >
              <div className={styles.taskItemTop}>
                <StatusDot status={statusOf(t)} title={t.enabled ? 'enabled' : 'disabled'} />
                <span className={styles.taskName}>{t.name}</span>
              </div>
              <div className={styles.taskMeta}>
                {triggerSummary(t.trigger)} · ran {ago(t.lastRun)}
              </div>
            </button>
          ))}
        </div>
      </aside>

      <section className={styles.detail}>
        {selectedId ? (
          <TaskDetail key={selectedId} id={selectedId} onDeleted={() => setSelectedId(null)} />
        ) : (
          <div className={styles.empty}>Select a task, or create one with “+ New”.</div>
        )}
      </section>
    </div>
  )
}

function TaskDetail({ id, onDeleted }: { id: string; onDeleted: () => void }) {
  const qc = useQueryClient()
  const [editMode, setEditMode] = useState(false)
  const [draft, setDraft] = useState<Task | null>(null)
  const [error, setError] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)

  const { data: task } = useQuery({ queryKey: ['task', id], queryFn: () => api.get<Task>(`/api/tasks/${id}`) })
  const { data: runs } = useQuery({
    queryKey: ['task-runs', id],
    queryFn: () => api.get<TaskRun[]>(`/api/tasks/${id}/runs?limit=20`),
    refetchInterval: 5000,
  })

  // New (empty) tasks open directly in edit mode (runs once when the task loads).
  useEffect(() => {
    if (task && task.rungs.length === 0) {
      setDraft(structuredClone(task))
      setEditMode(true)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [task?.id])

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['tasks'] })
    qc.invalidateQueries({ queryKey: ['task', id] })
    qc.invalidateQueries({ queryKey: ['task-runs', id] })
  }

  const run = useMutation({
    mutationFn: () => api.post(`/api/tasks/${id}/run`),
    onSuccess: () => { setError(''); invalidate() },
    onError: (e) => setError(isApiError(e) ? e.error : 'run failed'),
  })
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => api.post(`/api/tasks/${id}/${enabled ? 'enable' : 'disable'}`),
    onSuccess: invalidate,
  })
  const save = useMutation({
    mutationFn: (t: Task) => api.put<Task>(`/api/tasks/${id}`, t),
    onSuccess: () => { setError(''); setEditMode(false); invalidate() },
    onError: (e) => setError(isApiError(e) ? e.error : 'save failed'),
  })
  function doDelete() {
    fetch(`/api/tasks/${id}`, { method: 'DELETE', credentials: 'include' }).then(() => {
      setConfirmDelete(false)
      qc.invalidateQueries({ queryKey: ['tasks'] })
      onDeleted()
    })
  }

  if (!task) return <Loading text="loading task" />

  const startEdit = () => {
    setDraft(structuredClone(task))
    setEditMode(true)
    setError('')
  }
  const cancelEdit = () => {
    setEditMode(false)
    setDraft(null)
    setError('')
  }

  const view = editMode && draft ? draft : task

  return (
    <div className={styles.detailInner}>
      <header className={styles.detailHead}>
        <div className={styles.detailTitleWrap}>
          <StatusDot status={statusOf(task)} />
          {editMode && draft ? (
            <input
              className={styles.titleInput}
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            />
          ) : (
            <h2 className={styles.detailTitle}>{task.name}</h2>
          )}
        </div>
        <div className={styles.detailActions}>
          {!editMode && (
            <>
              <Button small disabled={run.isPending} onClick={() => run.mutate()}>
                {run.isPending ? 'running…' : 'Run now'}
              </Button>
              <Button small onClick={() => toggle.mutate(!task.enabled)}>
                {task.enabled ? 'Disable' : 'Enable'}
              </Button>
              <Button small onClick={startEdit}>
                Edit
              </Button>
              <Button small variant="danger" onClick={() => setConfirmDelete(true)}>
                Delete
              </Button>
            </>
          )}
          {editMode && draft && (
            <>
              <Button small variant="primary" disabled={save.isPending} onClick={() => save.mutate(draft)}>
                {save.isPending ? 'saving…' : 'Save'}
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
          <button className={styles.dismiss} onClick={() => setError('')}>✕</button>
        </div>
      )}

      <div className={styles.body}>
        <TriggerSection view={view} editMode={editMode} draft={draft} setDraft={setDraft} />

        <div className={styles.sectionLabel}>Ladder</div>
        {editMode && draft ? (
          <LadderEditor draft={draft} setDraft={setDraft} />
        ) : (
          <LadderView rungs={view.rungs} />
        )}

        {!editMode && (
          <>
            <div className={styles.sectionLabel}>Run history</div>
            <RunHistory runs={runs ?? []} />
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
            Delete <strong>{task.name}</strong> and its run history? This cannot be undone.
          </p>
        </Modal>
      )}
    </div>
  )
}

function TriggerSection({
  view,
  editMode,
  draft,
  setDraft,
}: {
  view: Task
  editMode: boolean
  draft: Task | null
  setDraft: (t: Task) => void
}) {
  if (!editMode || !draft) {
    return (
      <div className={styles.triggerRow}>
        <span className={styles.sectionLabel}>Trigger</span>
        <span className={styles.triggerVal}>{triggerSummary(view.trigger)}</span>
        {view.runAs && <span className={styles.muted}>run as {view.runAs}</span>}
      </div>
    )
  }
  const t = draft.trigger
  const setType = (type: TriggerType) => {
    const next = { ...draft, trigger: { ...t, type } }
    if (type === 'interval' && !next.trigger.intervalSeconds) next.trigger.intervalSeconds = 60
    if (type === 'cron' && !next.trigger.cron) next.trigger.cron = '*/5 * * * *'
    setDraft(next)
  }
  return (
    <div className={styles.triggerEdit}>
      <span className={styles.sectionLabel}>Trigger</span>
      <select className={styles.select} value={t.type} onChange={(e) => setType(e.target.value as TriggerType)}>
        <option value="manual">manual</option>
        <option value="interval">interval</option>
        <option value="cron">cron</option>
      </select>
      {t.type === 'interval' && (
        <input
          className={styles.numInput}
          type="number"
          min={1}
          value={t.intervalSeconds ?? 60}
          onChange={(e) => setDraft({ ...draft, trigger: { ...t, intervalSeconds: Number(e.target.value) } })}
        />
      )}
      {t.type === 'cron' && (
        <input
          className={styles.input}
          value={t.cron ?? ''}
          placeholder="*/5 * * * *"
          onChange={(e) => setDraft({ ...draft, trigger: { ...t, cron: e.target.value } })}
        />
      )}
      <input
        className={styles.input}
        value={draft.runAs ?? ''}
        placeholder="run as (user, optional)"
        onChange={(e) => setDraft({ ...draft, runAs: e.target.value })}
      />
    </div>
  )
}

function LadderView({ rungs }: { rungs: Rung[] }) {
  if (rungs.length === 0) return <div className={styles.muted}>no rungs — this task does nothing yet</div>
  return (
    <div className={styles.ladder}>
      {rungs.map((r, i) => (
        <div key={r.id ?? i} className={styles.rung}>
          <div className={styles.rungConds}>
            {r.contacts.length === 0 ? (
              <span className={`${styles.chip} ${styles.always}`}>ALWAYS</span>
            ) : (
              r.contacts.map((c, ci) => (
                <span key={c.id ?? ci} className={styles.condItem}>
                  {ci > 0 && <span className={styles.joiner}>{r.match === 'any' ? 'OR' : 'AND'}</span>}
                  <span className={`${styles.chip} ${c.negate ? styles.chipNeg : ''}`}>{contactLabel(c)}</span>
                </span>
              ))
            )}
          </div>
          <div className={styles.arrow}>→</div>
          <div className={styles.rungActs}>
            {r.actions.map((a, ai) => (
              <span key={a.id ?? ai} className={`${styles.chip} ${styles.coil}`}>
                {actionLabel(a)}
              </span>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function RunHistory({ runs }: { runs: TaskRun[] }) {
  const [open, setOpen] = useState<number | null>(null)
  if (runs.length === 0) return <div className={styles.muted}>never run</div>
  return (
    <div className={styles.runs}>
      {runs.map((r) => (
        <div key={r.id} className={styles.runRow}>
          <button className={styles.runHead} onClick={() => setOpen(open === r.id ? null : r.id)}>
            <StatusDot status={r.ok ? 'ok' : 'err'} />
            <span className={styles.runTime}>{new Date(r.time).toLocaleString()}</span>
            <span className={styles.muted}>{r.trigger}</span>
            <span className={styles.runSummary}>{r.summary}</span>
            <span className={styles.muted}>{r.durationMs}ms</span>
          </button>
          {open === r.id && r.detail && <pre className={styles.runDetail}>{formatDetail(r.detail)}</pre>}
        </div>
      ))}
    </div>
  )
}

function formatDetail(detail: string): string {
  try {
    const items = JSON.parse(detail) as { kind: string; detail?: string; ok: boolean; output?: string; error?: string }[]
    return items
      .map((it) => {
        const head = `[${it.ok ? 'ok' : 'ERR'}] ${it.kind}${it.detail ? ' · ' + it.detail : ''}`
        const out = it.output ? `\n  ${it.output.replace(/\n/g, '\n  ')}` : ''
        const err = it.error ? `\n  error: ${it.error}` : ''
        return head + out + err
      })
      .join('\n')
  } catch {
    return detail
  }
}

// ---------- editor ----------

function LadderEditor({ draft, setDraft }: { draft: Task; setDraft: (t: Task) => void }) {
  const patch = (mut: (d: Task) => void) => {
    const d = structuredClone(draft)
    mut(d)
    setDraft(d)
  }

  return (
    <div className={styles.editor}>
      {draft.rungs.map((rung, ri) => (
        <div key={rung.id ?? ri} className={styles.rungEdit}>
          <div className={styles.rungEditHead}>
            <span className={styles.rungNo}>Rung {ri + 1}</span>
            <select
              className={styles.select}
              value={rung.match}
              onChange={(e) => patch((d) => { d.rungs[ri].match = e.target.value as MatchMode })}
            >
              <option value="all">ALL (AND)</option>
              <option value="any">ANY (OR)</option>
            </select>
            <span style={{ flex: 1 }} />
            <Button small variant="ghost" onClick={() => patch((d) => { d.rungs.splice(ri, 1) })}>
              remove rung
            </Button>
          </div>

          <div className={styles.editCols}>
            <div className={styles.editCol}>
              <div className={styles.sectionLabel}>Conditions</div>
              {rung.contacts.map((c, ci) => (
                <div key={c.id ?? ci} className={styles.editRow}>
                  <label className={styles.negToggle}>
                    <input
                      type="checkbox"
                      checked={!!c.negate}
                      onChange={(e) => patch((d) => { d.rungs[ri].contacts[ci].negate = e.target.checked })}
                    />
                    not
                  </label>
                  <select
                    className={styles.select}
                    value={c.kind}
                    onChange={(e) => patch((d) => { const neg = d.rungs[ri].contacts[ci].negate; d.rungs[ri].contacts[ci] = defaultContact(e.target.value); d.rungs[ri].contacts[ci].negate = neg })}
                  >
                    {CONTACT_KINDS.map((k) => (
                      <option key={k} value={k}>{k}</option>
                    ))}
                  </select>
                  <ContactParams c={c} onParam={(k, v) => patch((d) => { d.rungs[ri].contacts[ci].params[k] = v })} />
                  <Button small variant="ghost" onClick={() => patch((d) => { d.rungs[ri].contacts.splice(ci, 1) })}>✕</Button>
                </div>
              ))}
              <AddSelect placeholder="+ condition…" options={CONTACT_KINDS} onAdd={(k) => patch((d) => { d.rungs[ri].contacts.push(defaultContact(k)) })} />
            </div>

            <div className={styles.editCol}>
              <div className={styles.sectionLabel}>Actions</div>
              {rung.actions.map((a, ai) => (
                <div key={a.id ?? ai} className={styles.editRow}>
                  <select
                    className={styles.select}
                    value={a.kind}
                    onChange={(e) => patch((d) => { d.rungs[ri].actions[ai] = defaultAction(e.target.value) })}
                  >
                    {ACTION_KINDS.map((k) => (
                      <option key={k} value={k}>{k}</option>
                    ))}
                  </select>
                  <ActionParams a={a} onParam={(k, v) => patch((d) => { d.rungs[ri].actions[ai].params[k] = v })} />
                  <Button small variant="ghost" onClick={() => patch((d) => { d.rungs[ri].actions.splice(ai, 1) })}>✕</Button>
                </div>
              ))}
              <AddSelect placeholder="+ action…" options={ACTION_KINDS} onAdd={(k) => patch((d) => { d.rungs[ri].actions.push(defaultAction(k)) })} />
            </div>
          </div>
        </div>
      ))}
      <Button small onClick={() => patch((d) => { d.rungs.push({ match: 'all', contacts: [], actions: [defaultAction('command')] }) })}>
        + rung
      </Button>
    </div>
  )
}

function AddSelect({ placeholder, options, onAdd }: { placeholder: string; options: string[]; onAdd: (k: string) => void }) {
  return (
    <select
      className={styles.addSelect}
      value=""
      onChange={(e) => {
        if (e.target.value) {
          onAdd(e.target.value)
          e.target.value = ''
        }
      }}
    >
      <option value="">{placeholder}</option>
      {options.map((k) => (
        <option key={k} value={k}>{k}</option>
      ))}
    </select>
  )
}

function ContactParams({ c, onParam }: { c: Contact; onParam: (k: string, v: unknown) => void }) {
  const p = c.params
  const text = (k: string, ph: string, w?: number) => (
    <input className={styles.input} style={w ? { width: w } : undefined} value={ps(p, k)} placeholder={ph} onChange={(e) => onParam(k, e.target.value)} />
  )
  switch (c.kind) {
    case 'service':
      return (
        <>
          {text('unit', 'unit.service')}
          <select className={styles.select} value={ps(p, 'state', 'active')} onChange={(e) => onParam('state', e.target.value)}>
            <option value="active">active</option>
            <option value="inactive">inactive</option>
            <option value="failed">failed</option>
          </select>
        </>
      )
    case 'process':
      return text('name', 'process name')
    case 'time':
      return (
        <>
          {text('start', 'HH:MM', 70)}
          <span className={styles.muted}>to</span>
          {text('end', 'HH:MM', 70)}
        </>
      )
    case 'metric':
      return (
        <>
          <select className={styles.select} value={ps(p, 'metric', 'cpu')} onChange={(e) => onParam('metric', e.target.value)}>
            {METRICS.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
          <select className={styles.select} value={ps(p, 'op', '>')} onChange={(e) => onParam('op', e.target.value)}>
            {OPS.map((o) => <option key={o} value={o}>{o}</option>)}
          </select>
          <input className={styles.numInput} type="number" value={pn(p, 'value', 0)} onChange={(e) => onParam('value', Number(e.target.value))} />
          {ps(p, 'metric') === 'disk' && text('mount', '/', 90)}
        </>
      )
    case 'file':
      return text('path', '/path/to/file')
    case 'flag':
      return text('name', 'flag name')
    case 'taskState':
      return (
        <>
          {text('task', 'task id')}
          <select className={styles.select} value={ps(p, 'state', 'enabled')} onChange={(e) => onParam('state', e.target.value)}>
            <option value="enabled">enabled</option>
            <option value="disabled">disabled</option>
          </select>
        </>
      )
    default:
      return null
  }
}

function ActionParams({ a, onParam }: { a: Action; onParam: (k: string, v: unknown) => void }) {
  const p = a.params
  const text = (k: string, ph: string, w?: number) => (
    <input className={styles.input} style={w ? { width: w } : undefined} value={ps(p, k)} placeholder={ph} onChange={(e) => onParam(k, e.target.value)} />
  )
  switch (a.kind) {
    case 'command':
      return (
        <>
          {text('command', 'shell command')}
          <input className={styles.numInput} type="number" value={pn(p, 'timeoutSeconds', 30)} title="timeout (s)" onChange={(e) => onParam('timeoutSeconds', Number(e.target.value))} />
        </>
      )
    case 'service':
      return (
        <>
          <select className={styles.select} value={ps(p, 'action', 'restart')} onChange={(e) => onParam('action', e.target.value)}>
            {SERVICE_ACTIONS.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
          {text('unit', 'unit.service')}
        </>
      )
    case 'flag':
      return (
        <>
          {text('name', 'flag name')}
          <label className={styles.negToggle}>
            <input type="checkbox" checked={pb(p, 'value')} onChange={(e) => onParam('value', e.target.checked)} />
            set
          </label>
        </>
      )
    case 'taskToggle':
      return (
        <>
          {text('task', 'task id')}
          <label className={styles.negToggle}>
            <input type="checkbox" checked={pb(p, 'enabled')} onChange={(e) => onParam('enabled', e.target.checked)} />
            enable
          </label>
        </>
      )
    case 'log':
      return text('message', 'message')
    default:
      return null
  }
}
