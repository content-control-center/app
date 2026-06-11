import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  cancelPost,
  getPost,
  postToPayload,
  updatePost,
  type CancelTarget,
} from '@/services/api/posts'
import type { Post, PostStatus } from '@/types/posts'

const SAVE_DEBOUNCE_MS = 600

// How often to refetch a `scheduled` post. While scheduled, the backend
// changes the status on its own (publisher auto-publish, or a cancel job
// landing an unschedule), so we poll to surface those without a reload.
const SCHEDULED_POLL_MS = 5_000

export const postKey = (id: string) => ['post', id] as const

export type TransitionStatusResult =
  | { ok: true; post: Post }
  | { ok: false; error: string }

type UsePostResult = {
  doc: Post | undefined
  changeDoc: (fn: (p: Post) => void) => void
  transitionStatus: (next: PostStatus) => Promise<TransitionStatusResult>
  // Requests cancellation of a Scheduled post via the cancel endpoint. The
  // status doesn't change synchronously — the poll above picks up the flip
  // once the worker confirms. Kept separate from transitionStatus so a
  // user-cancel is never executed as a plain status PUT (which would leave
  // the Zernio job running).
  cancelScheduled: (target: CancelTarget) => Promise<TransitionStatusResult>
  // True from the moment a cancellation is requested until the post
  // actually leaves `scheduled` (worker confirmed, or it published in the
  // race). Drives the "Unscheduling…" indicator and disables actions so a
  // second cancel job isn't enqueued while the first is in flight.
  cancelling: boolean
  loading: boolean
  error: Error | undefined
}

export function usePost(postId: string): UsePostResult {
  const qc = useQueryClient()
  const query = useQuery({
    queryKey: postKey(postId),
    queryFn: () => getPost(postId),
    enabled: !!postId,
    refetchInterval: (q) =>
      q.state.data?.status === 'scheduled' ? SCHEDULED_POLL_MS : false,
  })

  const [cancelling, setCancelling] = useState(false)
  const pendingRef = useRef<Post | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const genRef = useRef(0)

  const flush = useCallback(async () => {
    const next = pendingRef.current
    if (!next) return
    timerRef.current = null
    pendingRef.current = null
    const genAtFlush = genRef.current
    try {
      const saved = await updatePost(postId, postToPayload(next))
      if (genRef.current === genAtFlush) {
        qc.setQueryData(postKey(postId), saved)
      }
    } catch {
      qc.invalidateQueries({ queryKey: postKey(postId) })
    }
  }, [postId, qc])

  const changeDoc = useCallback(
    (fn: (p: Post) => void) => {
      const base = pendingRef.current ?? qc.getQueryData<Post>(postKey(postId))
      if (!base) return
      const next = structuredClone(base)
      fn(next)
      pendingRef.current = next
      genRef.current += 1
      qc.setQueryData(postKey(postId), next)
      if (timerRef.current) clearTimeout(timerRef.current)
      timerRef.current = setTimeout(() => {
        void flush()
      }, SAVE_DEBOUNCE_MS)
    },
    [postId, qc, flush],
  )

  // Status transitions skip the autosave debounce: they're committed
  // user actions, the server enforces a transition graph, and we want
  // to surface failures (invalid edge, missing platform) immediately.
  // We merge any pending autosave changes into the same PUT so a
  // half-typed title isn't lost when the user clicks "Schedule".
  const transitionStatus = useCallback(
    async (next: PostStatus): Promise<TransitionStatusResult> => {
      if (timerRef.current) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
      const base = pendingRef.current ?? qc.getQueryData<Post>(postKey(postId))
      if (!base) return { ok: false, error: 'Post not loaded' }
      pendingRef.current = null
      const optimistic = structuredClone(base)
      optimistic.status = next
      genRef.current += 1
      qc.setQueryData(postKey(postId), optimistic)
      try {
        const saved = await updatePost(postId, postToPayload(optimistic))
        qc.setQueryData(postKey(postId), saved)
        return { ok: true, post: saved }
      } catch (err) {
        qc.invalidateQueries({ queryKey: postKey(postId) })
        const message = err instanceof Error ? err.message : 'Unable to update post'
        return { ok: false, error: message }
      }
    },
    [postId, qc],
  )

  // Cancellation is asynchronous on the server: it enqueues a Zernio
  // cancel job and the post stays `scheduled` until the worker confirms,
  // then transitions to `target`. We don't optimistically flip the status
  // — the scheduled poll lands the real change. We do drop any pending
  // autosave so a debounced PUT can't race the cancel.
  const cancelScheduled = useCallback(
    async (target: CancelTarget): Promise<TransitionStatusResult> => {
      if (timerRef.current) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
      pendingRef.current = null
      genRef.current += 1
      setCancelling(true)
      try {
        await cancelPost(postId, target)
        const fresh = await getPost(postId)
        qc.setQueryData(postKey(postId), fresh)
        // Stay `cancelling`: fresh is still `scheduled` and the worker
        // hasn't landed the transition yet. The effect below clears it
        // once the status actually changes.
        return { ok: true, post: fresh }
      } catch (err) {
        setCancelling(false)
        const message = err instanceof Error ? err.message : 'Unable to unschedule post'
        return { ok: false, error: message }
      }
    },
    [postId, qc],
  )

  // Clear the unscheduling indicator once the post leaves `scheduled` —
  // either the cancel job landed the target status, or it published in the
  // race. Until then the poll keeps the badge on `scheduled`.
  const status = query.data?.status
  useEffect(() => {
    if (cancelling && status !== undefined && status !== 'scheduled') {
      setCancelling(false)
    }
  }, [cancelling, status])

  useEffect(() => {
    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current)
        timerRef.current = null
        void flush()
      }
    }
  }, [flush])

  return {
    doc: query.data,
    changeDoc,
    transitionStatus,
    cancelScheduled,
    cancelling,
    loading: query.isLoading,
    error: query.error ?? undefined,
  }
}
