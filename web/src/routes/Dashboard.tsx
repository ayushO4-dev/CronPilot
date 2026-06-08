import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { Button } from '../components/ui'
import { ChangePasswordForm } from '../components/ChangePasswordForm'
import styles from './Dashboard.module.css'

const TABS = [
  { to: '/overview', label: 'Overview' },
  { to: '/services', label: 'Services' },
  { to: '/applications', label: 'Applications' },
  { to: '/tasks', label: 'Tasks' },
  { to: '/terminal', label: 'Terminal' },
  { to: '/settings', label: 'Settings' },
]

export function Dashboard() {
  const { user, logout } = useAuth()
  const location = useLocation()
  const navRef = useRef<HTMLElement>(null)
  const [indicator, setIndicator] = useState<{ left: number; width: number } | null>(null)

  function measure() {
    const nav = navRef.current
    if (!nav) return
    const active = nav.querySelector<HTMLElement>('[aria-current="page"]')
    if (active) setIndicator({ left: active.offsetLeft, width: active.offsetWidth })
  }

  // Reposition the sliding underline whenever the active route changes.
  useLayoutEffect(measure, [location.pathname])

  // Keep it aligned if the header reflows.
  useEffect(() => {
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [])

  return (
    <div className={styles.shell}>
      <header className={styles.header}>
        <div className={styles.brand}>CronPilot</div>
        <nav className={styles.tabs} ref={navRef}>
          {TABS.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              className={({ isActive }) => (isActive ? `${styles.tab} ${styles.active}` : styles.tab)}
            >
              {t.label}
            </NavLink>
          ))}
          {indicator && (
            <span className={styles.indicator} style={{ left: indicator.left, width: indicator.width }} />
          )}
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
