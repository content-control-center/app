import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createTag, listTags } from '@/services/api/tags'

const TAGS_KEY = ['tags'] as const

export function useTags() {
  return useQuery({
    queryKey: TAGS_KEY,
    queryFn: listTags,
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
