import { useEffect, useRef, useState } from 'react'
import type { Sample } from './types'

const MAX_SAMPLES = 60

interface StreamState {
  latest: Sample | null
  history: Sample[]
  connected: boolean
}

// useSystemStream subscribes to the live metrics WebSocket, keeping a rolling
// window of recent samples. It auto-reconnects on drop.
export function useSystemStream(): StreamState {
  const [latest, setLatest] = useState<Sample | null>(null)
  const [history, setHistory] = useState<Sample[]>([])
  const [connected, setConnected] = useState(false)
  const stopped = useRef(false)

  useEffect(() => {
    stopped.current = false
    let ws: WebSocket | null = null
    let retry: number | undefined

    const connect = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      ws = new WebSocket(`${proto}://${location.host}/api/system/stream`)

      ws.onopen = () => setConnected(true)
      ws.onmessage = (e) => {
        try {
          const s = JSON.parse(e.data as string) as Sample
          setLatest(s)
          setHistory((h) => {
            const next = [...h, s]
            return next.length > MAX_SAMPLES ? next.slice(next.length - MAX_SAMPLES) : next
          })
        } catch {
          /* ignore malformed frame */
        }
      }
      ws.onclose = () => {
        setConnected(false)
        if (!stopped.current) retry = window.setTimeout(connect, 2000)
      }
      ws.onerror = () => ws?.close()
    }

    connect()
    return () => {
      stopped.current = true
      if (retry) clearTimeout(retry)
      ws?.close()
    }
  }, [])

  return { latest, history, connected }
}
