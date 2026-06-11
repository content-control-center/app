import type { Tag } from '@/types/content'
import { apiJson } from './http'

const BASE = '/api/tags'

export function listTags(): Promise<Tag[]> {
  return apiJson<Tag[]>(BASE, 'Unable to fetch tags')
}

export function createTag(payload: { name: string; color?: string }): Promise<Tag> {
  return apiJson<Tag>(BASE, 'Unable to create tag', { method: 'POST', body: payload })
}
