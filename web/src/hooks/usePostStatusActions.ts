import { useState } from 'react'
import type { Post, PostStatus } from '@/types/posts'
import {
  getActionMeta,
  getAllowedNextStatuses,
  getTransitionBlockers,
  isTerminalStatus,
  type PostStatusActionIntent,
  type PostStatusActionKind,
  type PostStatusBlocker,
} from '@/lib/postStatusMachine'
import type { TransitionStatusResult } from '@/hooks/usePost'

export type PostStatusAction = {
  next: PostStatus
  buttonLabel: string
  menuLabel: string
  intent: PostStatusActionIntent
  kind: PostStatusActionKind
  disabled: boolean
  blockers: PostStatusBlocker[]
  run: () => Promise<TransitionStatusResult>
}

type UsePostStatusActionsResult = {
  current: PostStatus
  isTerminal: boolean
  // All transitions defined in the machine for the current status,
  // including system-driven ones. UI typically filters to kind === 'user'.
  actions: PostStatusAction[]
  // True while any transition is in flight; gates concurrent clicks
  // since the underlying mutation owns the cache and only one PUT
  // should be in flight at a time.
  pending: boolean
  lastError: string | null
}

export function usePostStatusActions(
  post: Post,
  transitionStatus: (next: PostStatus) => Promise<TransitionStatusResult>,
): UsePostStatusActionsResult {
  const [pending, setPending] = useState(false)
  const [lastError, setLastError] = useState<string | null>(null)

  const actions: PostStatusAction[] = getAllowedNextStatuses(post.status).flatMap(
    (next) => {
      const meta = getActionMeta(post.status, next)
      if (!meta) return []
      const blockers = getTransitionBlockers(post, next)
      return [
        {
          next,
          buttonLabel: meta.buttonLabel,
          menuLabel: meta.menuLabel,
          intent: meta.intent,
          kind: meta.kind,
          disabled: blockers.length > 0 || pending,
          blockers,
          run: async () => {
            if (blockers.length > 0) {
              const message = blockers.map((b) => b.message).join('; ')
              setLastError(message)
              return { ok: false, error: message }
            }
            setPending(true)
            setLastError(null)
            const result = await transitionStatus(next)
            setPending(false)
            if (!result.ok) setLastError(result.error)
            return result
          },
        },
      ]
    },
  )

  return {
    current: post.status,
    isTerminal: isTerminalStatus(post.status),
    actions,
    pending,
    lastError,
  }
}
