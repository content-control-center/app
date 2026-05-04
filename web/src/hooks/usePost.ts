import { useCallback, useEffect, useRef } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getPost, postToPayload, updatePost } from '@/services/api/posts'
import type { Post, PostStatus } from '@/types/posts'

const SAVE_DEBOUNCE_MS = 600

export const postKey = (id: string) => ['post', id] as const

export type TransitionStatusResult =
  | { ok: true; post: Post }
  | { ok: false; error: string }

type UsePostResult = {
  doc: Post | undefined
  changeDoc: (fn: (p: Post) => void) => void
  transitionStatus: (next: PostStatus) => Promise<TransitionStatusResult>
  loading: boolean
  error: Error | undefined
}

export function usePost(postId: string): UsePostResult {
  const qc = useQueryClient()
  const query = useQuery({
    queryKey: postKey(postId),
    queryFn: () => getPost(postId),
    enabled: !!postId,
  })

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
    loading: query.isLoading,
    error: query.error ?? undefined,
  }
}
