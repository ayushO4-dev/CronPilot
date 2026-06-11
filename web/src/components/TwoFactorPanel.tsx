import { useState } from 'react'
import type { FormEvent } from 'react'
import { api, isApiError } from '../lib/api'
import { useAuth } from '../lib/auth'
import { Button, Field, Input } from './ui'

interface SetupData {
  secret: string
  url: string
  qr: string
}

const note = { color: 'var(--fg-muted)', fontSize: 'var(--fs-sm)' } as const
const errStyle = { color: 'var(--err)', fontSize: 'var(--fs-sm)' } as const
const okStyle = { color: 'var(--ok)', fontSize: 'var(--fs-sm)' } as const
const col = { display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', maxWidth: 380 } as const

export function TwoFactorPanel() {
  const { user, refresh } = useAuth()
  const enabled = user?.totpEnabled ?? false

  // Setup (enable) flow state.
  const [setup, setSetup] = useState<SetupData | null>(null)
  const [code, setCode] = useState('')
  // Disable flow state.
  const [password, setPassword] = useState('')

  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function beginSetup() {
    setError('')
    setBusy(true)
    try {
      setSetup(await api.post<SetupData>('/api/auth/totp/setup'))
      setCode('')
    } catch (err) {
      setError(isApiError(err) ? err.error : 'failed to start setup')
    } finally {
      setBusy(false)
    }
  }

  function cancelSetup() {
    setSetup(null)
    setCode('')
    setError('')
  }

  async function confirmEnable(e: FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await api.post('/api/auth/totp/enable', { code })
      await refresh()
      setSetup(null)
      setCode('')
    } catch (err) {
      setError(isApiError(err) ? err.error : 'failed to enable')
    } finally {
      setBusy(false)
    }
  }

  async function disable(e: FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await api.post('/api/auth/totp/disable', { password })
      await refresh()
      setPassword('')
    } catch (err) {
      setError(isApiError(err) ? err.error : 'failed to disable')
    } finally {
      setBusy(false)
    }
  }

  // Already on: show status + a password-gated disable.
  if (enabled) {
    return (
      <form onSubmit={disable} style={col}>
        <div style={okStyle}>Two-factor authentication is enabled.</div>
        <Field label="Current password" hint="required to turn 2FA off">
          <Input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
        </Field>
        {error && <div style={errStyle}>{error}</div>}
        <Button type="submit" variant="danger" disabled={busy}>
          {busy ? 'disabling…' : 'Disable 2FA'}
        </Button>
      </form>
    )
  }

  // Mid-setup: show QR + secret, confirm with a code.
  if (setup) {
    return (
      <form onSubmit={confirmEnable} style={col}>
        <div style={note}>
          Scan this QR code with an authenticator app (Google Authenticator, Aegis,
          1Password…), or enter the secret manually, then confirm with the 6-digit code.
        </div>
        <img
          src={setup.qr}
          alt="TOTP QR code"
          width={180}
          height={180}
          style={{ background: '#fff', padding: 'var(--space-2)', alignSelf: 'flex-start' }}
        />
        <Field label="Secret">
          <Input value={setup.secret} readOnly onFocus={(e) => e.currentTarget.select()} />
        </Field>
        <Field label="Authentication code" hint="6-digit code from the app">
          <Input
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]*"
            autoFocus
            required
          />
        </Field>
        {error && <div style={errStyle}>{error}</div>}
        <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? 'verifying…' : 'Confirm & enable'}
          </Button>
          <Button type="button" onClick={cancelSetup} disabled={busy}>
            Cancel
          </Button>
        </div>
      </form>
    )
  }

  // Off: offer to set up.
  return (
    <div style={col}>
      <div style={note}>
        Add a second factor: a time-based one-time code from an authenticator app,
        required at login in addition to your password.
      </div>
      {error && <div style={errStyle}>{error}</div>}
      <Button variant="primary" onClick={beginSetup} disabled={busy}>
        {busy ? 'starting…' : 'Set up 2FA'}
      </Button>
    </div>
  )
}
