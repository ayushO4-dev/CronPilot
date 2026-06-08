import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, isApiError } from '../../lib/api'
import type { ProcessDetail, ProcessInfo } from '../../lib/types'
import { Button, Input, Loading } from '../../components/ui'
import { Modal } from '../../components/Modal'
import { bytes, percent } from '../../lib/format'
import styles from './Applications.module.css'

type SortKey = 'pid' | 'name' | 'user' | 'cpuPercent' | 'memoryPercent' | 'rss' | 'status'
type SortDir = 'asc' | 'desc'

const COLUMNS: { key: SortKey; label: string; num?: boolean }[] = [
  { key: 'pid', label: 'PID', num: true },
  { key: 'name', label: 'Name' },
  { key: 'user', label: 'User' },
  { key: 'cpuPercent', label: 'CPU%', num: true },
  { key: 'memoryPercent', label: 'Mem%', num: true },
  { key: 'rss', label: 'RSS', num: true },
  { key: 'status', label: 'St' },
]

function compare(a: ProcessInfo, b: ProcessInfo, key: SortKey, dir: SortDir): number {
  const av = a[key]
  const bv = b[key]
  let r: number
  if (typeof av === 'number' && typeof bv === 'number') r = av - bv
  else r = String(av).localeCompare(String(bv))
  return dir === 'asc' ? r : -r
}

export function Applications() {
  const [q, setQ] = useState('')
  const [sortKey, setSortKey] = useState<SortKey>('cpuPercent')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [selected, setSelected] = useState<number | null>(null)

  const { data: procs, isLoading, isError } = useQuery({
    queryKey: ['processes'],
    queryFn: () => api.get<ProcessInfo[]>('/api/processes'),
    refetchInterval: 2500,
  })

  if (isLoading) return <Loading text="reading processes" />
  if (isError) return <div className={styles.error}>failed to load processes</div>

  const needle = q.toLowerCase()
  const rows = (procs ?? [])
    .filter((p) => {
      if (!needle) return true
      return (
        p.name.toLowerCase().includes(needle) ||
        p.cmdline.toLowerCase().includes(needle) ||
        p.user.toLowerCase().includes(needle) ||
        String(p.pid).includes(needle)
      )
    })
    .sort((a, b) => compare(a, b, sortKey, sortDir))

  function setSort(key: SortKey) {
    if (key === sortKey) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir(key === 'name' || key === 'user' || key === 'status' ? 'asc' : 'desc')
    }
  }

  const arrow = (key: SortKey) => (key === sortKey ? (sortDir === 'asc' ? ' ▲' : ' ▼') : '')

  return (
    <div className={styles.page}>
      <div className={styles.toolbar}>
        <Input placeholder="filter by name, command, user, pid…" value={q} onChange={(e) => setQ(e.target.value)} />
        <span className={styles.count}>{rows.length} processes</span>
      </div>

      <div className={styles.tableWrap}>
        <table>
          <thead>
            <tr>
              {COLUMNS.map((c) => (
                <th
                  key={c.key}
                  className={`${styles.sortable} ${c.num ? 'num' : ''}`}
                  onClick={() => setSort(c.key)}
                >
                  {c.label}
                  {arrow(c.key)}
                </th>
              ))}
              <th>Command</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => (
              <tr key={p.pid} onClick={() => setSelected(p.pid)}>
                <td className="num">{p.pid}</td>
                <td className={styles.name}>{p.name}</td>
                <td className={styles.muted}>{p.user}</td>
                <td className="num">{percent(p.cpuPercent, 1)}</td>
                <td className="num">{percent(p.memoryPercent, 1)}</td>
                <td className="num">{bytes(p.rss)}</td>
                <td className={styles.muted}>{p.status || '—'}</td>
                <td className={styles.cmd}>{p.cmdline || `[${p.name}]`}</td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr>
                <td colSpan={8} className={styles.muted}>
                  no matching processes
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {selected != null && <ProcessModal pid={selected} onClose={() => setSelected(null)} />}
    </div>
  )
}

function ProcessModal({ pid, onClose }: { pid: number; onClose: () => void }) {
  const qc = useQueryClient()
  const [err, setErr] = useState('')

  const { data: d, isError } = useQuery({
    queryKey: ['process', pid],
    queryFn: () => api.get<ProcessDetail>(`/api/processes/${pid}`),
    refetchInterval: 3000,
  })

  const signal = useMutation({
    mutationFn: (sig: string) => api.post(`/api/processes/${pid}/${sig}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['processes'] })
      onClose()
    },
    onError: (e) => setErr(isApiError(e) ? e.error : 'signal failed'),
  })

  const title = d ? `${d.name} (${pid})` : `pid ${pid}`

  const footer = (
    <>
      <Button small disabled={signal.isPending} onClick={() => signal.mutate('term')}>
        terminate
      </Button>
      <Button small disabled={signal.isPending} onClick={() => signal.mutate('hup')}>
        reload (HUP)
      </Button>
      <Button small variant="danger" disabled={signal.isPending} onClick={() => signal.mutate('kill')}>
        force kill
      </Button>
      <span className={styles.spacer} />
      <Button small onClick={onClose}>
        close
      </Button>
    </>
  )

  return (
    <Modal title={title} onClose={onClose} actions={footer}>
      {err && (
        <div className={styles.error}>
          <span>{err}</span>
          <button className={styles.dismiss} onClick={() => setErr('')} aria-label="dismiss">
            ✕
          </button>
        </div>
      )}
      {isError ? (
        <div className={styles.muted}>process no longer exists</div>
      ) : d ? (
        <table className={styles.detailTable}>
          <tbody>
            <tr>
              <th>PID / PPID</th>
              <td>
                {d.pid} / {d.ppid}
              </td>
            </tr>
            <tr>
              <th>User</th>
              <td>{d.user || '—'}</td>
            </tr>
            <tr>
              <th>Status</th>
              <td>{d.status || '—'}</td>
            </tr>
            <tr>
              <th>CPU / Mem</th>
              <td>
                {percent(d.cpuPercent, 1)} / {percent(d.memoryPercent, 1)} ({bytes(d.rss)})
              </td>
            </tr>
            <tr>
              <th>Threads / Nice</th>
              <td>
                {d.numThreads} / {d.nice}
              </td>
            </tr>
            <tr>
              <th>Executable</th>
              <td className={styles.path}>{d.exe || '—'}</td>
            </tr>
            <tr>
              <th>Working dir</th>
              <td className={styles.path}>{d.cwd || '—'}</td>
            </tr>
            <tr>
              <th>Command</th>
              <td className={styles.cmdFull}>{d.cmdline || `[${d.name}]`}</td>
            </tr>
          </tbody>
        </table>
      ) : (
        <Loading text="loading" />
      )}
    </Modal>
  )
}
