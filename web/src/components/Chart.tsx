import { useEffect, useRef } from 'react'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import styles from './Chart.module.css'

export interface ChartSeries {
  label: string
  color: string
}

// Chart is a thin wrapper over uPlot for compact live time-series. It rebuilds
// when the series structure changes and only pushes new data otherwise.
export function Chart({
  data,
  series,
  height = 120,
  yMax,
  yFmt,
}: {
  data: number[][] // [xs, ...seriesValues]
  series: ChartSeries[]
  height?: number
  yMax?: number
  yFmt?: (v: number) => string
}) {
  const elRef = useRef<HTMLDivElement>(null)
  const plotRef = useRef<uPlot | null>(null)

  useEffect(() => {
    const el = elRef.current
    if (!el) return

    const cs = getComputedStyle(document.documentElement)
    const muted = cs.getPropertyValue('--muted').trim() || '#888'
    const border = cs.getPropertyValue('--border').trim() || '#333'

    const opts: uPlot.Options = {
      width: el.clientWidth || 400,
      height,
      cursor: { x: true, y: false, points: { show: false } },
      legend: { show: false },
      scales: { x: { time: true }, y: yMax != null ? { range: [0, yMax] } : {} },
      axes: [
        {
          stroke: muted,
          grid: { stroke: border, width: 1 },
          ticks: { stroke: border, width: 1 },
          font: '10px monospace',
        },
        {
          stroke: muted,
          grid: { stroke: border, width: 1 },
          ticks: { stroke: border, width: 1 },
          font: '10px monospace',
          size: 52,
          values: yFmt ? (_u, splits) => splits.map((s) => yFmt(s)) : undefined,
        },
      ],
      series: [
        {},
        ...series.map((s) => ({ label: s.label, stroke: s.color, width: 1.5, points: { show: false } })),
      ],
    }

    plotRef.current = new uPlot(opts, data as uPlot.AlignedData, el)

    const ro = new ResizeObserver(() => {
      if (plotRef.current && elRef.current) {
        plotRef.current.setSize({ width: elRef.current.clientWidth, height })
      }
    })
    ro.observe(el)

    return () => {
      ro.disconnect()
      plotRef.current?.destroy()
      plotRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [series.length, height, yMax])

  useEffect(() => {
    plotRef.current?.setData(data as uPlot.AlignedData)
  }, [data])

  return <div ref={elRef} className={styles.chart} />
}
