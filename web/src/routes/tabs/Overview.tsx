import { useQuery } from "@tanstack/react-query";
import { api } from "../../lib/api";
import type { Summary } from "../../lib/types";
import { useMetrics } from "../../lib/metrics";
import { Chart } from "../../components/Chart";
import { Loading, Meter, Panel, Stat } from "../../components/ui";
import { bps, bytes, duration, percent } from "../../lib/format";
import styles from "./tabs.module.css";

function cssVar(name: string, fallback: string): string {
  const v = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  return v || fallback;
}

export function Overview() {
  const { data: summary } = useQuery({
    queryKey: ["summary"],
    queryFn: () => api.get<Summary>("/api/system/summary"),
    refetchInterval: 5000,
  });
  const { history, latest } = useMetrics();

  const accent = cssVar("--accent", "#4a9eff");
  const ok = cssVar("--ok", "#3fb950");
  const warn = cssVar("--warn", "#d29922");

  if (!summary) return <Loading text="reading system" />;

  const cpu = latest?.cpuPercent ?? summary.cpu.percent;
  const mem = latest?.memUsedPercent ?? summary.memory.usedPercent;
  const swap = latest?.swapUsedPercent ?? summary.swap.usedPercent;
  const disks = summary.disks ?? [];
  const perCore = latest?.perCore ?? summary.cpu.perCore ?? [];

  const xs = history.map((s) => s.time);
  const cpuData: number[][] = [xs, history.map((s) => s.cpuPercent)];
  const memData: number[][] = [xs, history.map((s) => s.memUsed)];
  const swapData: number[][] = [xs, history.map((s) => s.swapUsed)];
  const netData: number[][] = [
    xs,
    history.map((s) => s.netRxBytesPerSec),
    history.map((s) => s.netTxBytesPerSec),
  ];
  const gb = (v: number) => `${(v / 1024 ** 3).toFixed(1)} GB`;

  return (
    <div className={styles.dashPage}>
      <div className={styles.topRow}>
        <Panel>
          <Stat
            label="Host"
            value={summary.host.hostname || "—"}
            sub={`${summary.host.platform} ${summary.host.platformVersion}`.trim()}
          />
        </Panel>
        <Panel>
          <Stat
            label="Uptime"
            value={duration(summary.host.uptime)}
            sub={`${summary.host.procs} processes`}
          />
        </Panel>
        <Panel>
          <Stat
            label="Load 1m"
            value={summary.load ? summary.load.load1.toFixed(2) : "—"}
            sub={
              summary.load
                ? `5m ${summary.load.load5.toFixed(2)} · 15m ${summary.load.load15.toFixed(2)}`
                : ""
            }
          />
        </Panel>
        <Panel title="System">
          <table className={styles.sysTable}>
            <tbody>
              <tr>
                <th>Kernel</th>
                <td>
                  {summary.host.kernelVersion} ({summary.host.kernelArch})
                </td>
              </tr>
              <tr>
                <th>CPU</th>
                <td>{summary.cpu.modelName || "—"}</td>
              </tr>
              <tr>
                <th>Cores</th>
                <td>
                  {summary.cpu.cores} logical / {summary.cpu.physicalCores}{" "}
                  physical
                </td>
              </tr>
              <tr>
                <th>OS</th>
                <td>
                  {summary.host.os} · {summary.host.platform}{" "}
                  {summary.host.platformVersion}
                </td>
              </tr>
            </tbody>
          </table>
        </Panel>
      </div>

      <div className={styles.grid3}>
        <Panel title={`CPU  ${percent(cpu)}`}>
          <Chart
            data={cpuData}
            series={[{ label: "CPU %", color: accent }]}
            yMax={100}
            smooth
            area
            xAxis={false}
            yAxis={false}
          />
        </Panel>
        <Panel title={`Memory ${percent(mem)} · ${gb(summary.memory.total)}`}>
          <Chart
            data={memData}
            series={[{ label: "Mem", color: warn }]}
            yMax={summary.memory.total || undefined}
            yFmt={gb}
            smooth
            area
            xAxis={false}
          />
        </Panel>
        {summary.swap.total > 0 ? (
          <Panel title={`Swap ${percent(swap)} · ${gb(summary.swap.total)}`}>
            <Chart
              data={swapData}
              series={[{ label: "Swap", color: accent }]}
              yMax={summary.swap.total}
              yFmt={gb}
              smooth
              area
              xAxis={false}
            />
          </Panel>
        ) : (
          <Panel title="Swap">
            <div className={styles.muted}>not configured</div>
          </Panel>
        )}
        <Panel
          title={`Network  ↓ ${bps(latest?.netRxBytesPerSec ?? 0)}  ↑ ${bps(latest?.netTxBytesPerSec ?? 0)}`}
        >
          <Chart
            data={netData}
            series={[
              { label: "rx", color: ok },
              { label: "tx", color: accent },
            ]}
            yMinCeil={10 * 1024 * 1024}
            yFmt={(v) => `${bytes(v)}/s`}
            smooth
            area
            xAxis={false}
          />
        </Panel>
      </div>

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
    </div>
  );
}
