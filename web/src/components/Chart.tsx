import { useEffect, useRef } from 'react'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import styles from './Chart.module.css'

export interface ChartSeries {
  label: string
  color: string
}

// Chart is a thin wrapper over uPlot for compact live time-series. Usage charts
// typically run with smooth + area and no axes (the current value lives in the
// panel title); charts with a meaningful scale (e.g. bytes/s) keep the y-axis.
export function Chart({
  data,
  series,
  height = 120,
  yMax,
  yFmt,
  smooth = false,
  area = false,
  xAxis = true,
  yAxis = true,
}: {
  data: number[][] // [xs, ...seriesValues]
  series: ChartSeries[]
  height?: number
  yMax?: number
  yFmt?: (v: number) => string
  smooth?: boolean
  area?: boolean
  xAxis?: boolean
  yAxis?: boolean
}) {
  const elRef = useRef<HTMLDivElement>(null)
  const plotRef = useRef<uPlot | null>(null)

  useEffect(() => {
    const el = elRef.current
    if (!el) return

    const cs = getComputedStyle(document.documentElement)
    const muted = cs.getPropertyValue('--muted').trim() || '#888'
    const border = cs.getPropertyValue('--border').trim() || '#333'

    const splineFn = uPlot.paths?.spline
    const pathBuilder = smooth && splineFn ? splineFn() : undefined

    const opts: uPlot.Options = {
      width: el.clientWidth || 400,
      height: el.clientHeight || height,
      cursor: { x: true, y: false, points: { show: false } },
      legend: { show: false },
      scales: { x: { time: true }, y: yMax != null ? { range: [0, yMax] } : {} },
      axes: [
        {
          show: xAxis,
          stroke: muted,
          grid: { show: xAxis, stroke: border, width: 1 },
          ticks: { show: xAxis, stroke: border, width: 1 },
          font: '10px monospace',
        },
        {
          show: yAxis,
          stroke: muted,
          grid: { show: yAxis, stroke: border, width: 1 },
          ticks: { show: yAxis, stroke: border, width: 1 },
          font: '10px monospace',
          size: yAxis ? 52 : 0,
          values: yFmt ? (_u, splits) => splits.map((s) => yFmt(s)) : undefined,
        },
      ],
      series: [
        {},
        ...series.map((s) => ({
          label: s.label,
          stroke: s.color,
          width: 1.5,
          points: { show: false },
          ...(pathBuilder ? { paths: pathBuilder } : {}),
          ...(area ? { fill: fillFor(s.color) } : {}),
        })),
      ],
    }

    plotRef.current = new uPlot(opts, data as uPlot.AlignedData, el)

    const ro = new ResizeObserver(() => {
      if (plotRef.current && elRef.current) {
        plotRef.current.setSize({
          width: elRef.current.clientWidth || 400,
          height: elRef.current.clientHeight || height,
        })
      }
    })
    ro.observe(el)

    return () => {
      ro.disconnect()
      plotRef.current?.destroy()
      plotRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [series.length, height, yMax, smooth, area, xAxis, yAxis])

  useEffect(() => {
    plotRef.current?.setData(data as uPlot.AlignedData)
  }, [data])

  return <div ref={elRef} className={styles.chart} />
}

// fillFor turns a #rgb/#rrggbb stroke color into a translucent area fill.
function fillFor(color: string): string | undefined {
  const m = /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/.exec(color.trim())
  if (!m) return undefined
  let h = m[1]
  if (h.length === 3) {
    h = h
      .split('')
      .map((c) => c + c)
      .join('')
  }
  const r = parseInt(h.slice(0, 2), 16)
  const g = parseInt(h.slice(2, 4), 16)
  const b = parseInt(h.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, 0.15)`
}
