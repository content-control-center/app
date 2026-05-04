import type { PostStatus } from '@/types/posts'
import { POST_STATUS_LABELS } from '@/types/posts'
import { StatusBadge, type StatusTone } from '@/components/ui/status-badge'

const POST_STATUS_TONE: Record<PostStatus, StatusTone> = {
  draft: 'neutral',
  ready_for_publish: 'progress',
  scheduled: 'positive',
  scheduled_for_manual_publishing: 'warn',
  failed: 'destructive',
  published: 'positive',
  not_published: 'negative',
}

type Props = {
  status: PostStatus
  className?: string
}

export function PostStatusBadge({ status, className }: Props) {
  return (
    <StatusBadge
      label={POST_STATUS_LABELS[status] ?? status}
      tone={POST_STATUS_TONE[status]}
      className={className}
    />
  )
}
