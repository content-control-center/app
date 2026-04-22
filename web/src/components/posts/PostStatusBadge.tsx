import type { PostStatus } from '@/types/posts'
import { POST_STATUS_LABELS } from '@/types/posts'
import { cn } from '@/lib'

const STATUS_DOT_COLOR: Record<PostStatus, string> = {
  draft: 'bg-tertiary-foreground',
  ready_for_publish: 'bg-chart-4',
  scheduled: 'bg-positive',
  scheduled_for_manual_publishing: 'bg-chart-5',
  failed: 'bg-destructive',
  published: 'bg-positive',
  not_published: 'bg-negative',
}

type Props = {
  status: PostStatus
  className?: string
}

export function PostStatusBadge({ status, className }: Props) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 text-xs text-tertiary-foreground',
        className,
      )}
    >
      <span className={cn('size-1.5 rounded-full shrink-0', STATUS_DOT_COLOR[status])} />
      <span>{POST_STATUS_LABELS[status] ?? status}</span>
    </span>
  )
}
