import type { ReactElement } from 'react'
import type { Post } from '@/types/posts'
import { Button, buttonVariants } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'
import { cn } from '@/lib'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { PostStatusBadge } from '@/components/posts/PostStatusBadge'
import {
  usePostStatusActions,
  type PostStatusAction,
} from '@/hooks/usePostStatusActions'
import type { PostStatusBlocker } from '@/lib/postStatusMachine'
import type { TransitionStatusResult } from '@/hooks/usePost'

type Props = {
  post: Post
  transitionStatus: (next: Post['status']) => Promise<TransitionStatusResult>
}

const INTENT_RANK: Record<PostStatusAction['intent'], number> = {
  primary: 0,
  secondary: 1,
  destructive: 2,
}

const INTENT_VARIANT = {
  primary: 'default',
  secondary: 'outline',
  destructive: 'destructive',
} as const satisfies Record<PostStatusAction['intent'], string>

type IntentVariant = (typeof INTENT_VARIANT)[PostStatusAction['intent']]

const SCHEDULED_DATE_FORMAT = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  hour: 'numeric',
  minute: '2-digit',
})

export function PostStatusHeaderActions({ post, transitionStatus }: Props) {
  const { current, actions, pending } = usePostStatusActions(post, transitionStatus)

  // Hide system-driven transitions (e.g. publisher worker marking
  // `scheduled` → `published`); the user shouldn't trigger those.
  const userActions = actions
    .filter((a) => a.kind === 'user')
    .sort((a, b) => INTENT_RANK[a.intent] - INTENT_RANK[b.intent])

  const [primaryAction] = userActions
  // The dropdown lists every user action, including the one already
  // shown as the primary button — duplication is intentional so the
  // menu is a complete view of available transitions.
  const showOverflow = userActions.length > 1

  return (
    <div className="flex items-center gap-3">
      <PostStatusBadge status={current} />
      <ScheduleSummary post={post} />
      <div className="flex items-center gap-1">
        {showOverflow && primaryAction && (
          <OverflowMenu
            actions={userActions}
            pending={pending}
            variant={INTENT_VARIANT[primaryAction.intent]}
          />
        )}
        {primaryAction && (
          <PrimaryActionButton action={primaryAction} pending={pending} />
        )}
      </div>
    </div>
  )
}

// Wraps a disabled trigger in a span so a tooltip still fires on hover —
// disabled buttons don't emit pointer events themselves.
function BlockerTooltip({
  blockers,
  side,
  children,
}: {
  blockers: PostStatusBlocker[]
  side?: 'top' | 'right' | 'bottom' | 'left'
  children: ReactElement
}) {
  if (blockers.length === 0) return children
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0}>{children}</span>
      </TooltipTrigger>
      <TooltipContent side={side}>
        {blockers.map((b) => b.message).join(' · ')}
      </TooltipContent>
    </Tooltip>
  )
}

function PrimaryActionButton({
  action,
  pending,
}: {
  action: PostStatusAction
  pending: boolean
}) {
  return (
    <BlockerTooltip blockers={action.blockers}>
      <Button
        type="button"
        variant={INTENT_VARIANT[action.intent]}
        size="sm"
        disabled={action.disabled}
        loading={pending}
        onClick={() => {
          void action.run()
        }}
      >
        {action.buttonLabel}
      </Button>
    </BlockerTooltip>
  )
}

function OverflowMenu({
  actions,
  pending,
  variant,
}: {
  actions: PostStatusAction[]
  pending: boolean
  variant: IntentVariant
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className={cn(buttonVariants({ variant, size: 'smIcon' }))}
        aria-label="More status actions"
        disabled={pending}
      >
        <Icon name="dots_2_vertical" className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {actions.map((action) => (
          <BlockerTooltip key={action.next} blockers={action.blockers} side="left">
            <DropdownMenuItem
              variant={action.intent === 'destructive' ? 'destructive' : 'default'}
              disabled={action.disabled}
              onSelect={(e) => {
                if (action.disabled) {
                  e.preventDefault()
                  return
                }
                void action.run()
              }}
            >
              {action.menuLabel}
            </DropdownMenuItem>
          </BlockerTooltip>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function ScheduleSummary({ post }: { post: Post }) {
  if (post.status !== 'scheduled' && post.status !== 'scheduled_for_manual_publishing') {
    return null
  }
  if (!post.scheduled_at) {
    return <span className="text-xs text-tertiary-foreground">No date set</span>
  }
  const verb = post.status === 'scheduled' ? 'Auto-publishes' : 'Reminder'
  return (
    <span className="text-xs text-tertiary-foreground">
      {verb} {formatScheduledAt(post.scheduled_at)}
    </span>
  )
}

function formatScheduledAt(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return SCHEDULED_DATE_FORMAT.format(d)
}
