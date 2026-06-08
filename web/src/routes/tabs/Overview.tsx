import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { Summary } from '../../lib/types'
import { useSystemStream } from '../../lib/useSystemStream'
import { Loading, Meter, Panel, Stat } from '../../components/ui'
import { bps, bytes, duration, percent } from '../../lib/format'
import styles from './tabs.module.css'

export function Overview() {
  const { data: summary } = useQuery({
    queryKey: ['summary'],
    queryFn: () => api.get<Summary>('/api/system/summary'),
    refetchInterval: 5000,
  })
  const { latest } = useSystemStream()

  if (!summary) return <Loading text="reading system" />

  const cpu = latest?.cpuPercent ?? summary.cpu.percent
  const mem = latest?.memUsedPercent ?? summary.memory.usedPercent
  const disks = summary.disks ?? []
  const rootDisk = disks.find((d) => d.mountpoint === '/') ?? disks[0]

  return (
    <div className={styles.page}>
      <div className={styles.cards}>
        <Panel>
          <Stat
            label="Host"
            value={summary.host.hostname || '—'}
            sub={`${summary.host.platform} ${summary.host.platformVersion}`.trim()}
          />
        </Panel>
        <Panel>
          <Stat label="Uptime" value={duration(summary.host.uptime)} sub={`${summary.host.procs} processes`} />
        </Panel>
        <Panel>
          <div className={styles.meterRow}>
            <Stat label="CPU" value={percent(cpu)} sub={`${summary.cpu.cores} cores`} />
            <Meter value={cpu} />
          </div>
        </Panel>
        <Panel>
          <div className={styles.meterRow}>
            <Stat
              label="Memory"
              value={percent(mem)}
              sub={`${bytes(summary.memory.used)} / ${bytes(summary.memory.total)}`}
            />
            <Meter value={mem} />
          </div>
        </Panel>
        <Panel>
          <Stat
            label="Load (1m)"
            value={summary.load ? summary.load.load1.toFixed(2) : '—'}
            sub={summary.load ? `5m ${summary.load.load5.toFixed(2)} · 15m ${summary.load.load15.toFixed(2)}` : ''}
          />
        </Panel>
        {rootDisk && (
          <Panel>
            <div className={styles.meterRow}>
              <Stat
                label={`Disk ${rootDisk.mountpoint}`}
                value={percent(rootDisk.usedPercent)}
                sub={`${bytes(rootDisk.used)} / ${bytes(rootDisk.total)}`}
              />
              <Meter value={rootDisk.usedPercent} />
            </div>
          </Panel>
        )}
      </div>

      <div className={styles.grid2}>
        <Panel title="Network (live)">
          <table>
            <tbody>
              <tr>
                <th>Download</th>
                <td className="num">{bps(latest?.netRxBytesPerSec ?? 0)}</td>
              </tr>
              <tr>
                <th>Upload</th>
                <td className="num">{bps(latest?.netTxBytesPerSec ?? 0)}</td>
              </tr>
            </tbody>
          </table>
        </Panel>
        <Panel title="System">
          <table>
            <tbody>
              <tr>
                <th>Kernel</th>
                <td>
                  {summary.host.kernelVersion} ({summary.host.kernelArch})
                </td>
              </tr>
              <tr>
                <th>CPU</th>
                <td>{summary.cpu.modelName || '—'}</td>
              </tr>
              <tr>
                <th>Cores</th>
                <td>
                  {summary.cpu.cores} logical / {summary.cpu.physicalCores} physical
                </td>
              </tr>
            </tbody>
          </table>
        </Panel>
      </div>
    </div>
  )
}
