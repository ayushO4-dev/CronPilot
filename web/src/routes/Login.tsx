import { useState } from 'react'
import type { FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { isApiError } from '../lib/api'
import { Button, Field, Input } from '../components/ui'
import styles from './Login.module.css'

export function Login() {
  const { user, login } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (user) return <Navigate to="/overview" replace />

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await login(username, password)
      navigate('/overview', { replace: true })
    } catch (err) {
      setError(isApiError(err) ? err.error : 'login failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.card}>
        <div>
          <div className={styles.brand}>CronPilot</div>
          <div className={styles.subtitle}>Linux server manager</div>
        </div>
        <form className={styles.form} onSubmit={onSubmit}>
          <Field label="Username">
            <Input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              autoFocus
              required
            />
          </Field>
          <Field label="Password">
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
          </Field>
          {error && <div className={styles.error}>{error}</div>}
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? 'signing in…' : 'Sign in'}
          </Button>
        </form>
      </div>
    </div>
  )
}
