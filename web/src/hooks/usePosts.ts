import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import { createPost, deletePost, listCampaignPosts, updatePost } from '@/services/api/posts'
import type { Post, PostPayload } from '@/types/posts'

const campaignPostsKey = (campaignId: string) => ['campaigns', campaignId, 'posts'] as const

export function useCampaignPosts(campaignId: string) {
  return useQuery({
    queryKey: campaignPostsKey(campaignId),
    queryFn: () => listCampaignPosts(campaignId),
    enabled: !!campaignId,
  })
}

export function useCreatePost(campaignId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: PostPayload) => createPost(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: campaignPostsKey(campaignId) })
    },
  })
}

/**
 * Returns a handler that creates a blank post in the campaign and navigates to
 * its editor. Shared by the campaign header action and the empty-state buttons.
 */
export function useAddPost(campaignId: string) {
  const createPost = useCreatePost(campaignId)
  const navigate = useNavigate()
  return useCallback(() => {
    createPost.mutate(
      { campaign_id: campaignId },
      {
        onSuccess: (post) => {
          navigate({
            to: '/campaigns/$campaignId/posts/$postId',
            params: { campaignId, postId: post.id },
          })
        },
      },
    )
  }, [createPost, navigate, campaignId])
}

export function useUpdatePost(campaignId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: PostPayload }) =>
      updatePost(id, payload),
    onMutate: async ({ id, payload }) => {
      await qc.cancelQueries({ queryKey: campaignPostsKey(campaignId) })
      const prev = qc.getQueryData<Post[]>(campaignPostsKey(campaignId))
      if (prev) {
        qc.setQueryData<Post[]>(
          campaignPostsKey(campaignId),
          prev.map((p) => (p.id === id ? { ...p, ...payload } : p)),
        )
      }
      return { prev }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(campaignPostsKey(campaignId), ctx.prev)
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: campaignPostsKey(campaignId) })
    },
  })
}

export function useDeletePost(campaignId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deletePost(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: campaignPostsKey(campaignId) })
    },
  })
}
