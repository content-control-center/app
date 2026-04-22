import { useCallback, useEffect, useRef, useState } from 'react'
import type { FieldValues, UseFormReturn } from 'react-hook-form'
import { useUpdatePost } from '@/hooks/usePosts'
import type { Post, PostPayload } from '@/types/posts'

export function postToPayload(
  post: Post,
  overrides: Partial<PostPayload> = {},
): PostPayload {
  return {
    campaign_id: post.campaign_id,
    platform_id: post.platform_id,
    platform_post_type: post.platform_post_type,
    title: post.title,
    content: post.content,
    media_urls: post.media_urls,
    scheduled_at: post.scheduled_at,
    published_at: post.published_at,
    status: post.status,
    cta_type: post.cta_type,
    cta_url: post.cta_url,
    target_audience_notes: post.target_audience_notes,
    used_asset_ids: post.used_asset_ids,
    campaign_type_phase_id: post.campaign_type_phase_id,
    ...overrides,
  }
}

type UsePostAutosaveArgs<T extends FieldValues> = {
  post: Post
  form: UseFormReturn<T>
  buildOverrides: (values: T) => Partial<PostPayload>
  onFlushRef?: (flush: () => void) => void
}

export function usePostAutosave<T extends FieldValues>({
  post,
  form,
  buildOverrides,
  onFlushRef,
}: UsePostAutosaveArgs<T>) {
  const { mutate: updatePost, error, reset } = useUpdatePost(post.id, post.campaign_id)
  const editVersionRef = useRef(0)
  const [editVersion, setEditVersion] = useState(0)
  const [savedVersion, setSavedVersion] = useState(0)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const isDirty = editVersion !== savedVersion

  const saveNow = useCallback(() => {
    const v = editVersionRef.current
    void form.handleSubmit((values) => {
      const payload = postToPayload(post, buildOverrides(values))
      updatePost(payload, { onSuccess: () => setSavedVersion(v) })
    })()
  }, [post, form, updatePost, buildOverrides])

  const scheduleSave = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(saveNow, 500)
  }, [saveNow])

  const flushSave = useCallback(() => {
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current)
      saveTimerRef.current = null
      saveNow()
    }
  }, [saveNow])

  useEffect(() => {
    onFlushRef?.(flushSave)
  }, [onFlushRef, flushSave])

  useEffect(() => {
    const sub = form.watch(() => {
      editVersionRef.current += 1
      setEditVersion(editVersionRef.current)
      scheduleSave()
    })
    return () => sub.unsubscribe()
  }, [form, scheduleSave])

  useEffect(() => {
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    }
  }, [])

  return { isDirty, error, resetError: reset, flushSave }
}
