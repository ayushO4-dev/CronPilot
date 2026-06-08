import { useSyncExternalStore } from 'react'
import type { Sample } from './types'

const MAX = 90 // samples retained (~90s at a 1s cadence)

let history: Sample[] = []
let latest: Sample | null = null
let connected = false
let started = false
let ws: WebSocket | null = null

let snapshot: { latest: Sample | null; history: Sample[]; connected: boolean } = {
  latest,
  history,
  connected,
}
const listeners = new Set<() => void>()

function emit() {
  snapshot = { latest, history, connected }
  for (const l of listeners) l()
}

function push(s: Sample) {
  latest = s
  if (history.length === 0) {
    // Pre-seed a full window so the chart is immediately populated and scrolls
    // smoothly from the first reading. The time axis is hidden, so the
    // back-dated timestamps are never shown to the user.
    const seeded: Sample[] = []
    for (let i = MAX - 1; i >= 1; i--) seeded.push({ ...s, time: s.time - i })
    seeded.push(s)
    history = seeded
  } else {
    const next = history.concat(s)
    history = next.length > MAX ? next.slice(next.length - MAX) : next
  }
  emit()
}

function connect() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  ws = new WebSocket(`${proto}://${location.host}/api/system/stream`)
  ws.onopen = () => {
    connected = true
    emit()
  }
  ws.onmessage = (e) => {
    try {
      push(JSON.parse(e.data as string) as Sample)
    } catch {
      /* ignore malformed frame */
    }
  }
  ws.onclose = () => {
    connected = false
    emit()
    window.setTimeout(connect, 1500) // auto-reconnect; history is preserved
  }
  ws.onerror = () => ws?.close()
}

function ensureStarted() {
  if (started) return
  started = true
  connect()
}

const subscribe = (cb: () => void) => {
  ensureStarted()
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}
const getSnapshot = () => snapshot

// useMetrics returns live system metrics from a single shared WebSocket that
// stays connected across tab switches, so the charts are populated instantly
// (and keep filling) regardless of which tab is open.
export function useMetrics() {
  return useSyncExternalStore(subscribe, getSnapshot)
}
