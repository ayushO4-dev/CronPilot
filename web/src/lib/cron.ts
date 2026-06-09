// cronToEnglish translates a standard 5-field cron expression into a short
// plain-English description for common patterns, falling back to "custom
// schedule" for expressions it doesn't recognize.

const DAY_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

function pad(n: number): string {
  return n < 10 ? `0${n}` : `${n}`
}

function fmtTime(h: number, m: number): string {
  const ampm = h < 12 ? 'AM' : 'PM'
  let hh = h % 12
  if (hh === 0) hh = 12
  return `${hh}:${pad(m)} ${ampm}`
}

function dowLabel(dow: string): string | null {
  if (/^\d+$/.test(dow)) return DAY_NAMES[Number(dow) % 7]
  if (/^[0-7](,[0-7])+$/.test(dow)) {
    return dow.split(',').map((d) => DAY_NAMES[Number(d) % 7]).join(', ')
  }
  const range = /^(\d)-(\d)$/.exec(dow)
  if (range) {
    const a = Number(range[1])
    const b = Number(range[2])
    if (a === 1 && b === 5) return 'weekday'
    const out: string[] = []
    for (let i = a; i <= b; i++) out.push(DAY_NAMES[i % 7])
    return out.join(', ')
  }
  return null
}

export function cronToEnglish(expr: string): string {
  const s = expr.trim()
  if (!s) return 'enter a cron expression'
  const f = s.split(/\s+/)
  if (f.length !== 5) return 'custom schedule'
  const [min, hour, dom, mon, dow] = f
  const restEvery = hour === '*' && dom === '*' && mon === '*' && dow === '*'

  if (min === '*' && restEvery) return 'every minute'

  const mStep = /^\*\/(\d+)$/.exec(min)
  if (mStep && restEvery) {
    const n = Number(mStep[1])
    return n === 1 ? 'every minute' : `every ${n} minutes`
  }

  if (/^\d+$/.test(min) && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    const m = Number(min)
    return m === 0 ? 'at the start of every hour' : `at ${pad(m)} past every hour`
  }

  const hStep = /^\*\/(\d+)$/.exec(hour)
  if (/^\d+$/.test(min) && hStep && dom === '*' && mon === '*' && dow === '*') {
    const n = Number(hStep[1])
    const past = Number(min) === 0 ? '' : ` at ${pad(Number(min))} past the hour`
    return n === 1 ? `every hour${past}` : `every ${n} hours${past}`
  }

  if (/^\d+$/.test(min) && /^\d+$/.test(hour)) {
    const time = fmtTime(Number(hour), Number(min))
    if (dom === '*' && mon === '*' && dow !== '*') {
      const days = dowLabel(dow)
      if (days) return `every ${days} at ${time}`
    }
    if (/^\d+$/.test(dom) && mon === '*' && dow === '*') {
      return `on day ${dom} of every month at ${time}`
    }
    if (dom === '*' && mon === '*' && dow === '*') {
      return `every day at ${time}`
    }
  }

  return 'custom schedule'
}
