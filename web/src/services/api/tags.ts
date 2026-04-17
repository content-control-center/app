import type { Tag } from '@/types/content'

const BASE = '/api/tags'

export async function listTags(): Promise<Tag[]> {
  const res = await fetch(BASE, { method: 'GET', credentials: 'include' })
  if (!res.ok) throw new Error(await errorMessage(res, 'Unable to fetch tags'))
  return (await res.json()) as Tag[]
}

export async function createTag(payload: { name: string; color?: string }): Promise<Tag> {
  const res = await fetch(BASE, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) throw new Error(await errorMessage(res, 'Unable to create tag'))
  return (await res.json()) as Tag
}

async function errorMessage(res: Response, fallback: string): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string }
    if (typeof body.error === 'string' && body.error.length > 0) return body.error
  } catch {
    // fall through
  }
  return fallback
}
