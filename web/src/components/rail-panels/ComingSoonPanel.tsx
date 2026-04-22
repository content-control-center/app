import { RailPanel } from '@/components/page-primitives/RailPanel'

type Props = {
  title: string
  onClose?: () => void
}

export function ComingSoonPanel({ title, onClose }: Props) {
  return (
    <RailPanel
      title={title}
      onClose={onClose}
      className="h-full"
      bodyClassName="flex-1 items-center justify-center"
    >
      <span className="text-sm text-tertiary-foreground">Coming soon</span>
    </RailPanel>
  )
}

export function AIAssistantPanel({ onClose }: { onClose?: () => void }) {
  return <ComingSoonPanel title="AI assistant" onClose={onClose} />
}

export function StatsPanel({ onClose }: { onClose?: () => void }) {
  return <ComingSoonPanel title="Stats" onClose={onClose} />
}
