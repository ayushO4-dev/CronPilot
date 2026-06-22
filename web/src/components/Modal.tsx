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
  rightPanel,
  rightPanelOpen,
}: {
  title: ReactNode
  onClose: () => void
  children: ReactNode
  actions?: ReactNode
  width?: number
  // Optional floating panel rendered beside the modal (e.g. a file editor). The
  // modal + panel are centered as a group, so showing it shifts the modal left.
  rightPanel?: ReactNode
  // Drives the open/close width+fade transition of rightPanel.
  rightPanelOpen?: boolean
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
      <div className={styles.group}>
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
        {rightPanel && (
          <div
            className={`${styles.rightPanel} ${rightPanelOpen ? styles.rightPanelOpen : ''}`}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className={styles.rightPanelInner}>{rightPanel}</div>
          </div>
        )}
      </div>
    </div>
  )
}
