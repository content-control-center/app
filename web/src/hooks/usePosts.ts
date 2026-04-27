import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createPost, deletePost, listCampaignPosts } from '@/services/api/posts'
import type { PostPayload } from '@/types/posts'

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

export function useDeletePost(campaignId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deletePost(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: campaignPostsKey(campaignId) })
    },
  })
}
