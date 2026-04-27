import { useCallback, useEffect, useRef } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getPost, postToPayload, updatePost } from '@/services/api/posts'
import type { Post } from '@/types/posts'

const SAVE_DEBOUNCE_MS = 600

export const postKey = (id: string) => ['post', id] as const

type UsePostResult = {
  doc: Post | undefined
  changeDoc: (fn: (p: Post) => void) => void
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
    loading: query.isLoading,
    error: query.error ?? undefined,
  }
}
