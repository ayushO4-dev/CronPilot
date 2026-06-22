import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { api, isApiError } from '../../lib/api'
import type { TerminalUser } from '../../lib/types'
import { useTerminalSession } from '../../lib/terminalSession'
import { Button, Input } from '../../components/ui'
import styles from './Terminal.module.css'

interface TermSession {
  ticket: string
  user: string
}

export function Terminal() {
  const active = useTerminalSession((s) => s.active)
  const endedUser = useTerminalSession((s) => s.endedUser)
  const start = useTerminalSession((s) => s.start)

  return (
    <div className={styles.wrap}>
      {active ? (
        <TerminalView />
      ) : (
        <AccountPicker
          endedUser={endedUser}
          onConnect={(s) => start(s.user, s.ticket)}
        />
      )}
    </div>
  )
}

// AccountPicker is a full-screen overlay (covering the tab content) for
// choosing which system account the shell logs in as. Root always requires its
// password up front; other non-daemon accounts authenticate inside the
// terminal via su's own prompt.
function AccountPicker({
  endedUser,
  onConnect,
}: {
  endedUser: string | null
  onConnect: (s: TermSession) => void
}) {
  const { data: users, isLoading } = useQuery({
    queryKey: ['terminal-users'],
    queryFn: () => api.get<TerminalUser[]>('/api/terminal/users'),
  })
  const [sel, setSel] = useState<string | null>(null)
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  // Default to the daemon's own account once the list arrives.
  useEffect(() => {
    if (!sel && users && users.length > 0) {
      const cur = users.find((u) => u.current)
      setSel((cur ?? users[0]).name)
    }
  }, [users, sel])

  const selected = users?.find((u) => u.name === sel)
  const needsPassword = selected?.name === 'root'
  const canConnect = !!selected && !busy && (!needsPassword || password.length > 0)

  // Sliding selection indicator on the left of the account list.
  const listRef = useRef<HTMLDivElement>(null)
  const [indicator, setIndicator] = useState<{ top: number; height: number } | null>(null)
  useLayoutEffect(() => {
    const c = listRef.current
    if (!c || !sel) {
      setIndicator(null)
      return
    }
    const el = c.querySelector<HTMLElement>(`[data-user="${sel}"]`)
    setIndicator(el ? { top: el.offsetTop, height: el.offsetHeight } : null)
  }, [sel, users])

  async function connect(e?: FormEvent) {
    e?.preventDefault()
    if (!selected || !canConnect) return
    setBusy(true)
    setError('')
    try {
      const res = await api.post<{ ticket: string }>('/api/terminal/session', {
        user: selected.name,
        password: needsPassword ? password : '',
      })
      setPassword('')
      onConnect({ ticket: res.ticket, user: selected.name })
    } catch (err) {
      setError(isApiError(err) ? err.error : 'failed to start session')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={styles.overlay}>
      <form className={styles.picker} onSubmit={connect}>
        <div className={styles.pickerTitle}>Terminal login</div>
        <div className={styles.pickerSub}>
          {endedUser ? `session as ${endedUser} ended — start a new one` : 'select the account to log in as'}
        </div>

        {isLoading && <div className={styles.pickerNote}>reading accounts…</div>}

        <div className={styles.userList} ref={listRef}>
          {indicator && (
            <span className={styles.userIndicator} style={{ top: indicator.top, height: indicator.height }} />
          )}
          {(users ?? []).map((u) => (
            <button
              key={u.name}
              type="button"
              data-user={u.name}
              className={`${styles.userRow} ${sel === u.name ? styles.userRowSel : ''}`}
              onClick={() => {
                setSel(u.name)
                setError('')
              }}
            >
              <span className={styles.userName}>{u.name}</span>
              <span className={styles.userMeta}>
                uid {u.uid}
                {u.current ? ' · daemon account' : ''}
                {u.name === 'root' ? ' · password required' : ''}
              </span>
            </button>
          ))}
        </div>

        {needsPassword && (
          <Input
            type="password"
            placeholder="root password"
            value={password}
            autoFocus
            onChange={(e) => {
              setPassword(e.target.value)
              setError('')
            }}
          />
        )}

        {selected && !selected.current && selected.name !== 'root' && (
          <div className={styles.pickerNote}>su will ask for {selected.name}'s password inside the terminal</div>
        )}

        {error && <div className={styles.pickerError}>{error}</div>}

        <Button type="submit" variant="primary" disabled={!canConnect}>
          {busy ? 'connecting…' : 'Connect'}
        </Button>
      </form>
    </div>
  )
}

function TerminalView() {
  const containerRef = useRef<HTMLDivElement>(null)
  const end = useTerminalSession((s) => s.end)
  const consumeTicket = useTerminalSession((s) => s.consumeTicket)
  // Bumping gen re-runs the effect to reconnect after an unexpected drop.
  const [gen, setGen] = useState(0)

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const cs = getComputedStyle(document.documentElement)
    const term = new XTerm({
      fontFamily: cs.getPropertyValue('--font-mono').trim() || 'monospace',
      fontSize: 14,
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

    // A ticket is only present on the first connect (it starts the shell).
    // Reconnects omit it and reattach to the still-running session.
    const ticket = consumeTicket()
    const q = new URLSearchParams({ cols: String(term.cols), rows: String(term.rows) })
    if (ticket) q.set('ticket', ticket)
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/api/terminal?${q.toString()}`)
    ws.binaryType = 'arraybuffer'

    let disposed = false
    let endedByShell = false
    let reconnectTimer: number | undefined

    ws.onopen = () => term.focus()
    ws.onmessage = (e) => {
      if (typeof e.data === 'string') {
        // Text frames are control messages; everything else is a plain notice.
        try {
          const msg = JSON.parse(e.data)
          if (msg && msg.type === 'ended') {
            endedByShell = true
            return
          }
        } catch {
          /* not JSON — fall through and print it */
        }
        term.write(e.data)
      } else {
        term.write(new Uint8Array(e.data as ArrayBuffer))
      }
    }
    ws.onclose = () => {
      if (disposed) return
      if (endedByShell) {
        // The shell itself exited (e.g. `exit`): drop back to the picker.
        end()
        return
      }
      // Unexpected drop or tab switch: try to resume the live session.
      term.write('\r\n\x1b[33m[reconnecting…]\x1b[0m\r\n')
      reconnectTimer = window.setTimeout(() => {
        if (!disposed) setGen((g) => g + 1)
      }, 1000)
    }

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
      disposed = true
      if (reconnectTimer) window.clearTimeout(reconnectTimer)
      ro.disconnect()
      dataDisp.dispose()
      resizeDisp.dispose()
      ws.close()
      term.dispose()
    }
  }, [gen, consumeTicket, end])

  return <div ref={containerRef} className={styles.term} />
}
