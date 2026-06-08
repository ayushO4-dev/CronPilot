import { Panel } from '../../components/ui'

export function Placeholder({ title, phase }: { title: string; phase: number }) {
  return (
    <Panel title={title}>
      <div style={{ color: 'var(--muted)', padding: 'var(--space-4)', fontSize: 'var(--fs-sm)' }}>
        {title} is not implemented yet — planned for Phase {phase}.
      </div>
    </Panel>
  )
}
