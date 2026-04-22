import { memo } from 'react'
import { Link } from '@tanstack/react-router'
import type { Post } from '@/types/posts'
import { POST_STATUS_LABELS } from '@/types/posts'
import { cn } from '@/lib'

type PostCardProps = {
  post: Post
}

const STATUS_DOT_COLOR: Record<string, string> = {
  draft: 'bg-tertiary-foreground',
  ready_for_publish: 'bg-chart-4',
  scheduled: 'bg-positive',
  scheduled_for_manual_publishing: 'bg-chart-5',
  failed: 'bg-destructive',
  published: 'bg-positive',
  not_published: 'bg-negative',
}

function PostCardComponent({ post }: PostCardProps) {
  const title = !post.title || post.title.trim() === '' ? 'Untitled' : post.title
  const platform = post.platform?.name ?? '—'
  const statusLabel = POST_STATUS_LABELS[post.status] ?? post.status
  const dotColor = STATUS_DOT_COLOR[post.status] ?? 'bg-tertiary-foreground'

  return (
    <Link
      to="/campaigns/$campaignId/posts/$postId"
      params={{ campaignId: post.campaign_id, postId: post.id }}
      className={cn(
        'bg-primary px-3 py-2 min-w-60 max-w-90',
        'flex flex-col gap-1 cursor-pointer hover:bg-secondary transition-colors',
      )}
    >
      <div className={'flex justify-between'}>
        <div className="flex items-center gap-1.5 text-xs text-tertiary-foreground">
          <div className="flex items-center gap-2 text-xs text-tertiary-foreground">
            <span>{platform}</span>
          </div>
        </div>
        <div className="flex items-center gap-1.5 text-xs text-tertiary-foreground">
          <span className={cn('size-1.5 rounded-full shrink-0', dotColor)} />
          <span>{statusLabel}</span>
        </div>
      </div>

      <div className="text-sm font-medium truncate">{title}</div>
    </Link>
  )
}

export const PostCard = memo(PostCardComponent)
