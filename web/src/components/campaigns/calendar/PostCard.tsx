import { memo } from 'react'
import { Link } from '@tanstack/react-router'
import { CircleDashed, Clock } from '@phosphor-icons/react'
import type { Post } from '@/types/posts'
import { POST_STATUS_LABELS } from '@/types/posts'
import { cn, formatTitle } from '@/lib'
import { getPlatformInfo, getPostTypeLabel } from '@/lib/platformDictionary'

type PostCardProps = {
  post: Post
}

// Status is conveyed by a 2px accent on the card's left edge. Draft is
// intentionally transparent (the border still reserves its 2px so every
// card stays aligned).
const STATUS_BORDER_COLOR: Record<string, string> = {
  draft: 'border-l-transparent',
  ready_for_publish: 'border-l-chart-4',
  scheduled: 'border-l-positive',
  scheduled_for_manual_publishing: 'border-l-chart-5',
  failed: 'border-l-destructive',
  published: 'border-l-positive',
  not_published: 'border-l-negative',
}

function PostCardComponent({ post }: PostCardProps) {
  const title = formatTitle(post.title)
  const platformInfo = getPlatformInfo(post.platform_id)
  // Fall back to a neutral, "undefined"-feeling dashed circle (in the muted
  // tertiary color, not a warning hue) when no platform is assigned.
  const PlatformIcon = platformInfo?.icon ?? CircleDashed
  const label = platformInfo
    ? getPostTypeLabel(post.platform_id, post.platform_post_type)
    : 'No platform'
  const statusLabel = POST_STATUS_LABELS[post.status] ?? post.status
  const borderColor = STATUS_BORDER_COLOR[post.status] ?? 'border-l-transparent'
  // The calendar lays posts out by scheduled_at; show that time (or the
  // publish time once published). Unscheduled posts have neither — omit it.
  const timeSource = post.scheduled_at ?? post.published_at
  const time = timeSource
    ? new Date(timeSource).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      })
    : null

  return (
    <Link
      to="/campaigns/$campaignId/posts/$postId"
      params={{ campaignId: post.campaign_id, postId: post.id }}
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData('text/plain', post.id)
        e.dataTransfer.effectAllowed = 'move'
      }}
      className={cn(
        'bg-primary px-3 py-2 w-full border-l-2',
        borderColor,
        'flex flex-col gap-1 cursor-pointer hover:bg-secondary transition-colors',
      )}
    >
      {/* Row 1 — status */}
      <div className="text-xs text-tertiary-foreground truncate">{statusLabel}</div>

      {/* Row 2 — title, up to two lines */}
      <div className="text-base font-medium line-clamp-2">{title}</div>

      {/* Row 3 — time */}
      {time && (
        <div className="flex items-center gap-1.5 text-xs text-tertiary-foreground">
          <Clock className="size-3.5 shrink-0" />
          <span className="truncate">{time}</span>
        </div>
      )}

      {/* Row 4 — platform */}
      <div className="flex items-center gap-1.5 text-xs text-tertiary-foreground min-w-0">
        <PlatformIcon
          weight="fill"
          color={platformInfo?.color}
          className="size-3.5 shrink-0"
        />
        <span className="truncate">{label}</span>
      </div>
    </Link>
  )
}

export const PostCard = memo(PostCardComponent)
