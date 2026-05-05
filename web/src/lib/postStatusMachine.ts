import type { Post, PostStatus } from '@/types/posts'

// Mirrors src/models/post.go ValidPostTransitions. Keep in sync with the
// server — the server is the source of truth and will reject any edge
// not listed here with a 400.
export const POST_STATUS_TRANSITIONS: Record<PostStatus, PostStatus[]> = {
  draft: ['ready_for_publish'],
  ready_for_publish: ['scheduled', 'scheduled_for_manual_publishing', 'draft'],
  scheduled: ['failed', 'published'],
  scheduled_for_manual_publishing: ['published', 'not_published'],
  failed: ['ready_for_publish'],
  published: [],
  not_published: ['ready_for_publish', 'scheduled_for_manual_publishing'],
}

export function getAllowedNextStatuses(current: PostStatus): PostStatus[] {
  return POST_STATUS_TRANSITIONS[current] ?? []
}

export function canTransition(from: PostStatus, to: PostStatus): boolean {
  if (from === to) return true
  return getAllowedNextStatuses(from).includes(to)
}

export function isTerminalStatus(status: PostStatus): boolean {
  return getAllowedNextStatuses(status).length === 0
}

export type PostStatusActionIntent = 'primary' | 'secondary' | 'destructive'

// `kind: 'system'` means the transition is normally driven by a backend
// process (e.g. the publisher worker marks `scheduled` → `published`),
// not by the user clicking a button. The UI should hide these by
// default but they're listed here so the machine is complete.
export type PostStatusActionKind = 'user' | 'system'

type ActionMeta = {
  // ALL CAPS form, used as the prominent header button label.
  buttonLabel: string
  // Sentence-case form, used inside the overflow dropdown menu.
  menuLabel: string
  intent: PostStatusActionIntent
  kind: PostStatusActionKind
}

const ACTION_META: Record<PostStatus, Partial<Record<PostStatus, ActionMeta>>> = {
  draft: {
    ready_for_publish: {
      buttonLabel: 'MARK AS READY',
      menuLabel: 'Mark as ready',
      intent: 'primary',
      kind: 'user',
    },
  },
  ready_for_publish: {
    scheduled: {
      buttonLabel: 'SCHEDULE',
      menuLabel: 'Schedule',
      intent: 'primary',
      kind: 'user',
    },
    scheduled_for_manual_publishing: {
      buttonLabel: 'SCHEDULE',
      menuLabel: 'Schedule for manual publish',
      intent: 'primary',
      kind: 'user',
    },
    draft: {
      buttonLabel: 'BACK TO DRAFT',
      menuLabel: 'Back to draft',
      intent: 'secondary',
      kind: 'user',
    },
  },
  scheduled: {
    published: {
      buttonLabel: 'MARK AS PUBLISHED',
      menuLabel: 'Mark as published',
      intent: 'primary',
      kind: 'system',
    },
    failed: {
      buttonLabel: 'MARK AS FAILED',
      menuLabel: 'Mark as failed',
      intent: 'destructive',
      kind: 'system',
    },
  },
  scheduled_for_manual_publishing: {
    published: {
      buttonLabel: 'MARK AS PUBLISHED',
      menuLabel: 'Mark as published',
      intent: 'primary',
      kind: 'user',
    },
    not_published: {
      buttonLabel: 'MARK AS NOT PUBLISHED',
      menuLabel: 'Mark as not published',
      intent: 'destructive',
      kind: 'user',
    },
  },
  failed: {
    ready_for_publish: {
      buttonLabel: 'RETRY TO PUBLISH',
      menuLabel: 'Retry to publish',
      intent: 'primary',
      kind: 'user',
    },
  },
  published: {},
  not_published: {
    scheduled_for_manual_publishing: {
      buttonLabel: 'RESCHEDULE',
      menuLabel: 'Reschedule',
      intent: 'primary',
      kind: 'user',
    },
    ready_for_publish: {
      buttonLabel: 'MOVE TO READY FOR PUBLISH',
      menuLabel: 'Move to ready for publish',
      intent: 'secondary',
      kind: 'user',
    },
  },
}

export function getActionMeta(from: PostStatus, to: PostStatus): ActionMeta | null {
  return ACTION_META[from]?.[to] ?? null
}

export type PostStatusBlocker = {
  field: 'platform_id' | 'platform_post_type'
  message: string
}

// Mirrors requirePlatformIfNotDraft in src/handlers/posts.go: any non-draft
// status requires both platform fields. Returns blockers the UI should
// show before letting the user attempt the transition; the server will
// also reject with 400 if these are missing.
export function getTransitionBlockers(post: Post, next: PostStatus): PostStatusBlocker[] {
  const blockers: PostStatusBlocker[] = []
  if (next !== 'draft') {
    if (!post.platform_id) {
      blockers.push({ field: 'platform_id', message: 'Pick a platform first' })
    }
    if (!post.platform_post_type) {
      blockers.push({ field: 'platform_post_type', message: 'Pick a post type first' })
    }
  }
  return blockers
}
