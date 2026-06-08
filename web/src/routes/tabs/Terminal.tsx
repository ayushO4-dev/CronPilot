import { useEffect, useRef } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import styles from './Terminal.module.css'

export function Terminal() {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const cs = getComputedStyle(document.documentElement)
    const term = new XTerm({
      fontFamily: cs.getPropertyValue('--font-mono').trim() || 'monospace',
      fontSize: 13,
      cursorBlink: true,
      theme: {
        background: cs.getPropertyValue('--bg').trim() || '#0b0c0e',
        foreground: cs.getPropertyValue('--fg').trim() || '#cfd6dd',
        cursor: cs.getPropertyValue('--accent').trim() || '#4a9eff',
        selectionBackground: cs.getPropertyValue('--selection').trim() || 'rgba(74,158,255,0.28)',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(container)
    fit.fit()

    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/api/terminal?cols=${term.cols}&rows=${term.rows}`)
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => term.focus()
    ws.onmessage = (e) => {
      if (typeof e.data === 'string') term.write(e.data)
      else term.write(new Uint8Array(e.data as ArrayBuffer))
    }
    ws.onclose = () => term.write('\r\n\x1b[31m[disconnected]\x1b[0m\r\n')

    const dataDisp = term.onData((d) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'input', data: d }))
    })
    const resizeDisp = term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols, rows }))
    })

    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
      } catch {
        /* container not measurable yet */
      }
    })
    ro.observe(container)

    return () => {
      ro.disconnect()
      dataDisp.dispose()
      resizeDisp.dispose()
      ws.close()
      term.dispose()
    }
  }, [])

  return (
    <div className={styles.wrap}>
      <div ref={containerRef} className={styles.term} />
    </div>
  )
}
