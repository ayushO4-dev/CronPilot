import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from 'react'
import styles from './ui.module.css'

type ButtonVariant = 'default' | 'primary' | 'danger' | 'ghost'

export function Button({
  variant = 'default',
  small,
  className,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; small?: boolean }) {
  return (
    <button
      className={[styles.btn, styles[variant], small ? styles.small : '', className ?? ''].join(' ')}
      {...rest}
    />
  )
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={styles.input} {...props} />
}

export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: string
  hint?: string
  error?: string
  children: ReactNode
}) {
  return (
    <label className={styles.field}>
      <span className={styles.fieldLabel}>{label}</span>
      {children}
      {error ? (
        <span className={styles.fieldError}>{error}</span>
      ) : hint ? (
        <span className={styles.fieldHint}>{hint}</span>
      ) : null}
    </label>
  )
}

export function Panel({
  title,
  actions,
  children,
  className,
}: {
  title?: ReactNode
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={[styles.panel, className ?? ''].join(' ')}>
      {(title || actions) && (
        <header className={styles.panelHead}>
          <h3 className={styles.panelTitle}>{title}</h3>
          {actions && <div className={styles.panelActions}>{actions}</div>}
        </header>
      )}
      <div className={styles.panelBody}>{children}</div>
    </section>
  )
}

export type Status = 'ok' | 'warn' | 'err' | 'muted'

export function StatusDot({ status, title }: { status: Status; title?: string }) {
  return <span className={[styles.dot, styles[`dot_${status}`]].join(' ')} title={title} />
}

export function Meter({ value }: { value: number }) {
  const v = Math.max(0, Math.min(100, value))
  const level = v >= 90 ? 'err' : v >= 70 ? 'warn' : 'ok'
  return (
    <div className={styles.meter}>
      <div className={[styles.meterFill, styles[`meter_${level}`]].join(' ')} style={{ width: `${v}%` }} />
    </div>
  )
}

export function Stat({ label, value, sub }: { label: string; value: ReactNode; sub?: ReactNode }) {
  return (
    <div className={styles.stat}>
      <div className={styles.statLabel}>{label}</div>
      <div className={styles.statValue}>{value}</div>
      {sub != null && <div className={styles.statSub}>{sub}</div>}
    </div>
  )
}

export function Loading({ text = 'loading' }: { text?: string }) {
  return <div className={styles.loading}>{text}…</div>
}
