import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { Summary } from '../../lib/types'
import { useSystemStream } from '../../lib/useSystemStream'
import { Chart } from '../../components/Chart'
import { Meter, Panel } from '../../components/ui'
import { bytes, percent } from '../../lib/format'
import styles from './tabs.module.css'

function cssVar(name: string, fallback: string): string {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

export function Monitor() {
  const { data: summary } = useQuery({
    queryKey: ['summary'],
    queryFn: () => api.get<Summary>('/api/system/summary'),
    refetchInterval: 5000,
  })
  const { history, latest, connected } = useSystemStream()

  const accent = cssVar('--accent', '#4a9eff')
  const ok = cssVar('--ok', '#3fb950')
  const warn = cssVar('--warn', '#d29922')

  const xs = history.map((s) => s.time)
  const cpuData: number[][] = [xs, history.map((s) => s.cpuPercent)]
  const memData: number[][] = [xs, history.map((s) => s.memUsedPercent)]
  const netData: number[][] = [
    xs,
    history.map((s) => s.netRxBytesPerSec),
    history.map((s) => s.netTxBytesPerSec),
  ]

  const perCore = latest?.perCore ?? summary?.cpu.perCore ?? []
  const disks = summary?.disks ?? []

  return (
    <div className={styles.page}>
      <div className={styles.grid2}>
        <Panel title={`CPU  ${latest ? percent(latest.cpuPercent) : ''}`}>
          <Chart data={cpuData} series={[{ label: 'CPU %', color: accent }]} yMax={100} yFmt={(v) => `${v.toFixed(0)}%`} />
        </Panel>
        <Panel title={`Memory  ${latest ? percent(latest.memUsedPercent) : ''}`}>
          <Chart data={memData} series={[{ label: 'Mem %', color: warn }]} yMax={100} yFmt={(v) => `${v.toFixed(0)}%`} />
        </Panel>
      </div>

      <Panel title="Network throughput">
        <Chart
          data={netData}
          series={[
            { label: 'rx', color: ok },
            { label: 'tx', color: accent },
          ]}
          height={140}
          yFmt={(v) => `${bytes(v)}/s`}
        />
      </Panel>

      <div className={styles.grid2}>
        <Panel title="Per-core utilization">
          <div className={styles.cores}>
            {perCore.map((c, i) => (
              <div key={i} className={styles.core}>
                <span className={styles.coreLabel}>{i}</span>
                <Meter value={c} />
                <span className={styles.corePct}>{percent(c, 0)}</span>
              </div>
            ))}
          </div>
        </Panel>
        <Panel title="Filesystems">
          <table>
            <thead>
              <tr>
                <th>Mount</th>
                <th>Type</th>
                <th className="num">Used</th>
                <th className="num">Size</th>
                <th className="num">%</th>
              </tr>
            </thead>
            <tbody>
              {disks.map((d) => (
                <tr key={d.mountpoint}>
                  <td>{d.mountpoint}</td>
                  <td className={styles.muted}>{d.fstype}</td>
                  <td className="num">{bytes(d.used)}</td>
                  <td className="num">{bytes(d.total)}</td>
                  <td className="num">{percent(d.usedPercent, 0)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>
      </div>

      <div className={styles.statusline}>{connected ? 'live' : 'reconnecting…'}</div>
    </div>
  )
}
