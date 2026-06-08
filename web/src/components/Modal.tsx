import { useEffect } from 'react'
import type { ReactNode } from 'react'
import styles from './Modal.module.css'

// Modal is a centered popup with a header, scrollable body, and optional footer.
// Closes on Escape or clicking the backdrop.
export function Modal({
  title,
  onClose,
  children,
  actions,
  width,
}: {
  title: ReactNode
  onClose: () => void
  children: ReactNode
  actions?: ReactNode
  width?: number
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className={styles.overlay} onMouseDown={onClose}>
      <div className={styles.modal} style={width ? { width } : undefined} onMouseDown={(e) => e.stopPropagation()}>
        <header className={styles.head}>
          <h2 className={styles.title}>{title}</h2>
          <button className={styles.close} onClick={onClose} aria-label="close">
            ✕
          </button>
        </header>
        <div className={styles.body}>{children}</div>
        {actions && <footer className={styles.foot}>{actions}</footer>}
      </div>
    </div>
  )
}
