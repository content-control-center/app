import { cn } from '@/lib'

export type StatusTone =
  | 'neutral'
  | 'positive'
  | 'negative'
  | 'destructive'
  | 'progress'
  | 'warn'

const TONE_DOT: Record<StatusTone, string> = {
  neutral: 'bg-tertiary-foreground',
  positive: 'bg-positive',
  negative: 'bg-negative',
  destructive: 'bg-destructive',
  progress: 'bg-chart-4',
  warn: 'bg-chart-5',
}

type Props = {
  label: string
  tone: StatusTone
  className?: string
}

export function StatusBadge({ label, tone, className }: Props) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 text-xs text-tertiary-foreground',
        className,
      )}
    >
      <span className={cn('size-1.5 rounded-full shrink-0', TONE_DOT[tone])} />
      <span>{label}</span>
    </span>
  )
}
