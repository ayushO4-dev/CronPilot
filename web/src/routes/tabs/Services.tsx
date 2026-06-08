import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, isApiError } from '../../lib/api'
import type { ServiceDetail, ServiceUnit } from '../../lib/types'
import { Button, Input, Loading, Panel, StatusDot } from '../../components/ui'
import type { Status } from '../../components/ui'
import { bytes } from '../../lib/format'
import styles from './Services.module.css'

function statusOf(active: string): Status {
  switch (active) {
    case 'active':
      return 'ok'
    case 'failed':
      return 'err'
    case 'activating':
    case 'deactivating':
    case 'reloading':
      return 'warn'
    default:
      return 'muted'
  }
}

const ACTIONS = ['start', 'stop', 'restart'] as const
type Filter = 'all' | 'active' | 'failed'

export function Services() {
  const qc = useQueryClient()
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [selected, setSelected] = useState<string | null>(null)
  const [error, setError] = useState('')

  const { data: units, isLoading } = useQuery({
    queryKey: ['services'],
    queryFn: () => api.get<ServiceUnit[]>('/api/services'),
    refetchInterval: 5000,
  })

  const action = useMutation({
    mutationFn: ({ name, act }: { name: string; act: string }) =>
      api.post(`/api/services/${encodeURIComponent(name)}/${act}`),
    onSuccess: (_data, vars) => {
      setError('')
      qc.invalidateQueries({ queryKey: ['services'] })
      qc.invalidateQueries({ queryKey: ['service', vars.name] })
      qc.invalidateQueries({ queryKey: ['service-logs', vars.name] })
    },
    onError: (e) => setError(isApiError(e) ? e.error : 'action failed'),
  })

  if (isLoading) return <Loading text="reading services" />

  const needle = q.toLowerCase()
  const filtered = (units ?? []).filter((u) => {
    if (needle && !u.name.toLowerCase().includes(needle) && !u.description.toLowerCase().includes(needle)) {
      return false
    }
    if (filter === 'active' && u.activeState !== 'active') return false
    if (filter === 'failed' && u.activeState !== 'failed') return false
    return true
  })

  return (
    <div className={styles.page}>
      {error && (
        <div className={styles.error}>
          <span>{error}</span>
          <button className={styles.dismiss} onClick={() => setError('')} aria-label="dismiss">
            ✕
          </button>
        </div>
      )}

      <div className={styles.toolbar}>
        <Input placeholder="filter services…" value={q} onChange={(e) => setQ(e.target.value)} />
        <div className={styles.filters}>
          {(['all', 'active', 'failed'] as const).map((f) => (
            <Button key={f} small variant={filter === f ? 'primary' : 'default'} onClick={() => setFilter(f)}>
              {f}
            </Button>
          ))}
        </div>
        <span className={styles.count}>{filtered.length} units</span>
      </div>

      <Panel className={styles.tablePanel}>
        <div className={styles.tableWrap}>
          <table>
            <thead>
              <tr>
                <th></th>
                <th>Unit</th>
                <th>Active</th>
                <th>Sub</th>
                <th>Enabled</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((u) => (
                <tr
                  key={u.name}
                  className={selected === u.name ? styles.selectedRow : undefined}
                  onClick={() => setSelected(u.name)}
                >
                  <td>
                    <StatusDot status={statusOf(u.activeState)} title={u.activeState} />
                  </td>
                  <td className={styles.unitCell}>
                    <div className={styles.unitName}>{u.name}</div>
                    <div className={styles.desc}>{u.description}</div>
                  </td>
                  <td>{u.activeState}</td>
                  <td className={styles.muted}>{u.subState}</td>
                  <td className={styles.muted}>{u.enabled || '—'}</td>
                  <td className={styles.actions} onClick={(e) => e.stopPropagation()}>
                    {ACTIONS.map((a) => (
                      <Button key={a} small disabled={action.isPending} onClick={() => action.mutate({ name: u.name, act: a })}>
                        {a}
                      </Button>
                    ))}
                    {u.enabled === 'enabled' ? (
                      <Button small disabled={action.isPending} onClick={() => action.mutate({ name: u.name, act: 'disable' })}>
                        disable
                      </Button>
                    ) : (
                      <Button small disabled={action.isPending} onClick={() => action.mutate({ name: u.name, act: 'enable' })}>
                        enable
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={6} className={styles.muted}>
                    no matching units
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Panel>

      {selected && <ServiceDetailPanel name={selected} onClose={() => setSelected(null)} />}
    </div>
  )
}

function ServiceDetailPanel({ name, onClose }: { name: string; onClose: () => void }) {
  const { data: detail } = useQuery({
    queryKey: ['service', name],
    queryFn: () => api.get<ServiceDetail>(`/api/services/${encodeURIComponent(name)}`),
  })
  const { data: logs } = useQuery({
    queryKey: ['service-logs', name],
    queryFn: () => api.get<{ lines: string[] | null }>(`/api/services/${encodeURIComponent(name)}/logs?lines=200`),
    refetchInterval: 5000,
  })

  return (
    <Panel
      title={name}
      actions={
        <Button small onClick={onClose}>
          close
        </Button>
      }
      className={styles.detail}
    >
      {detail && (
        <table className={styles.detailTable}>
          <tbody>
            <tr>
              <th>Description</th>
              <td>{detail.description || '—'}</td>
            </tr>
            <tr>
              <th>Load</th>
              <td>{detail.loadState}</td>
            </tr>
            <tr>
              <th>Active</th>
              <td>
                {detail.activeState} ({detail.subState})
              </td>
            </tr>
            <tr>
              <th>Enabled</th>
              <td>{detail.enabled || '—'}</td>
            </tr>
            <tr>
              <th>Main PID</th>
              <td>{detail.mainPID || '—'}</td>
            </tr>
            <tr>
              <th>Memory</th>
              <td>{detail.memoryCurrent ? bytes(detail.memoryCurrent) : '—'}</td>
            </tr>
            <tr>
              <th>Since</th>
              <td>{detail.since || '—'}</td>
            </tr>
            <tr>
              <th>Unit file</th>
              <td className={styles.path}>{detail.fragmentPath || '—'}</td>
            </tr>
          </tbody>
        </table>
      )}
      <div className={styles.logsHead}>Recent logs</div>
      <pre className={styles.logs}>{logs?.lines && logs.lines.length > 0 ? logs.lines.join('\n') : 'no log entries'}</pre>
    </Panel>
  )
}
