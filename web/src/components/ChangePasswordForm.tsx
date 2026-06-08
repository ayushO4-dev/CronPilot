import { useState } from 'react'
import type { FormEvent } from 'react'
import { api, isApiError } from '../lib/api'
import { useAuth } from '../lib/auth'
import { Button, Field, Input } from './ui'

export function ChangePasswordForm({ onSuccess }: { onSuccess?: () => void }) {
  const { refresh } = useAuth()
  const [oldPassword, setOld] = useState('')
  const [newPassword, setNew] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [ok, setOk] = useState(false)
  const [busy, setBusy] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setOk(false)
    if (newPassword !== confirm) {
      setError('new passwords do not match')
      return
    }
    setBusy(true)
    try {
      await api.post('/api/auth/change-password', { oldPassword, newPassword })
      await refresh()
      setOk(true)
      setOld('')
      setNew('')
      setConfirm('')
      onSuccess?.()
    } catch (err) {
      setError(isApiError(err) ? err.error : 'failed to change password')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={onSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', maxWidth: 380 }}>
      <Field label="Current password">
        <Input
          type="password"
          value={oldPassword}
          onChange={(e) => setOld(e.target.value)}
          autoComplete="current-password"
          required
        />
      </Field>
      <Field label="New password" hint="≥12 chars; 3+ of upper, lower, digit, symbol">
        <Input
          type="password"
          value={newPassword}
          onChange={(e) => setNew(e.target.value)}
          autoComplete="new-password"
          required
        />
      </Field>
      <Field label="Confirm new password">
        <Input
          type="password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          autoComplete="new-password"
          required
        />
      </Field>
      {error && <div style={{ color: 'var(--err)', fontSize: 'var(--fs-sm)' }}>{error}</div>}
      {ok && <div style={{ color: 'var(--ok)', fontSize: 'var(--fs-sm)' }}>password changed</div>}
      <Button type="submit" variant="primary" disabled={busy}>
        {busy ? 'saving…' : 'Change password'}
      </Button>
    </form>
  )
}
