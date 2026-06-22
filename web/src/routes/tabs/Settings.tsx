import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { Settings as SettingsT } from '../../lib/types'
import { useAuth } from '../../lib/auth'
import { initialTheme, saveTheme } from '../../lib/theme'
import type { Theme } from '../../lib/theme'
import { Button, Panel } from '../../components/ui'
import { ChangePasswordForm } from '../../components/ChangePasswordForm'
import { TwoFactorPanel } from '../../components/TwoFactorPanel'
import { UpdatePanel } from '../../components/UpdatePanel'
import { duration } from '../../lib/format'
import styles from './tabs.module.css'

export function Settings() {
  const { logout } = useAuth()
  const [theme, setTheme] = useState<Theme>(initialTheme())
  const { data } = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsT>('/api/settings') })

  function changeTheme(t: Theme) {
    setTheme(t)
    void saveTheme(t)
  }

  return (
    <div className={styles.page}>
      <Panel title="Appearance">
        <div className={styles.row}>
          <span>Theme</span>
          <div className={styles.themeBtns}>
            <Button small variant={theme === 'dark' ? 'primary' : 'default'} onClick={() => changeTheme('dark')}>
              Dark
            </Button>
            <Button small variant={theme === 'light' ? 'primary' : 'default'} onClick={() => changeTheme('light')}>
              Light
            </Button>
          </div>
        </div>
      </Panel>

      <Panel title="Change password">
        <ChangePasswordForm />
      </Panel>

      <Panel title="Two-factor authentication">
        <TwoFactorPanel />
      </Panel>

      <Panel title="Software update">
        <UpdatePanel />
      </Panel>

      <Panel title="Server">
        <table>
          <tbody>
            <tr>
              <th>Version</th>
              <td>{data?.version ?? '—'}</td>
            </tr>
            <tr>
              <th>Mode</th>
              <td>{data ? (data.dev ? 'development' : 'production') : '—'}</td>
            </tr>
            <tr>
              <th>Session idle timeout</th>
              <td>{data ? duration(data.sessionIdleSeconds) : '—'}</td>
            </tr>
            <tr>
              <th>Session max lifetime</th>
              <td>{data ? duration(data.sessionMaxSeconds) : '—'}</td>
            </tr>
          </tbody>
        </table>
      </Panel>

      <Panel title="Session">
        <Button variant="danger" onClick={() => void logout()}>
          Logout
        </Button>
      </Panel>
    </div>
  )
}
