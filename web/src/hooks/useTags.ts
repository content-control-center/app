import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createTag, listTags } from '@/services/api/tags'

export const TAGS_KEY = ['tags'] as const

const FIVE_MINUTES = 1000 * 60 * 5;

export function useTags() {
  return useQuery({
    queryKey: TAGS_KEY,
    queryFn: listTags,
    staleTime: FIVE_MINUTES,
  })
}

export function useCreateTag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name: string; color?: string }) => createTag(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: TAGS_KEY })
    },
  })
}
