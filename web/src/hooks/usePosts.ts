import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listCampaignPosts, getPost, createPost, updatePost, deletePost } from '@/services/api/posts'
import type { PostPayload } from '@/types/posts'

const campaignPostsKey = (campaignId: string) => ['campaigns', campaignId, 'posts'] as const
const postKey = (postId: string) => ['posts', postId] as const

export function useCampaignPosts(campaignId: string) {
  return useQuery({
    queryKey: campaignPostsKey(campaignId),
    queryFn: () => listCampaignPosts(campaignId),
    enabled: !!campaignId,
  })
}

export function usePost(postId: string) {
  return useQuery({
    queryKey: postKey(postId),
    queryFn: () => getPost(postId),
    enabled: !!postId,
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

export function useUpdatePost(postId: string, campaignId?: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: PostPayload) => updatePost(postId, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: postKey(postId) })
      if (campaignId) {
        qc.invalidateQueries({ queryKey: campaignPostsKey(campaignId) })
      }
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
