import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { Button } from '../components/ui'
import { ChangePasswordForm } from '../components/ChangePasswordForm'
import styles from './Dashboard.module.css'

const TABS = [
  { to: '/overview', label: 'Overview' },
  { to: '/monitor', label: 'Monitor' },
  { to: '/services', label: 'Services' },
  { to: '/applications', label: 'Applications' },
  { to: '/tasks', label: 'Tasks' },
  { to: '/terminal', label: 'Terminal' },
  { to: '/settings', label: 'Settings' },
]

export function Dashboard() {
  const { user, logout } = useAuth()

  return (
    <div className={styles.shell}>
      <header className={styles.header}>
        <div className={styles.brand}>CronPilot</div>
        <nav className={styles.tabs}>
          {TABS.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              className={({ isActive }) => (isActive ? `${styles.tab} ${styles.active}` : styles.tab)}
            >
              {t.label}
            </NavLink>
          ))}
        </nav>
        <div className={styles.user}>
          <span className={styles.username}>{user?.username}</span>
          <Button small onClick={() => void logout()}>
            Logout
          </Button>
        </div>
      </header>

      <main className={styles.main}>
        <Outlet />
      </main>

      {user?.mustChangePassword && (
        <div className={styles.overlay}>
          <div className={styles.modal}>
            <h2 className={styles.modalTitle}>Change your password</h2>
            <p className={styles.modalText}>You must set a new password before continuing.</p>
            <ChangePasswordForm />
          </div>
        </div>
      )}
    </div>
  )
}
