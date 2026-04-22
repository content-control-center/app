import type { Post, PostPayload } from '@/types/posts'

const BASE = '/api/posts'

export async function listCampaignPosts(campaignId: string): Promise<Post[]> {
  const res = await fetch(`/api/campaigns/${campaignId}/posts`, {
    method: 'GET',
    credentials: 'include',
  })
  if (!res.ok) {
    throw new Error(await errorMessage(res, 'Unable to fetch posts'))
  }
  return (await res.json()) as Post[]
}

export async function getPost(id: string): Promise<Post> {
  const res = await fetch(`${BASE}/${id}`, {
    method: 'GET',
    credentials: 'include',
  })
  if (!res.ok) {
    throw new Error(await errorMessage(res, 'Unable to fetch post'))
  }
  return (await res.json()) as Post
}

export async function createPost(payload: PostPayload): Promise<Post> {
  const res = await fetch(BASE, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    throw new Error(await errorMessage(res, 'Unable to create post'))
  }
  return (await res.json()) as Post
}

export async function updatePost(id: string, payload: PostPayload): Promise<Post> {
  const res = await fetch(`${BASE}/${id}`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    throw new Error(await errorMessage(res, 'Unable to update post'))
  }
  return (await res.json()) as Post
}

export async function deletePost(id: string): Promise<void> {
  const res = await fetch(`${BASE}/${id}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok) {
    throw new Error(await errorMessage(res, 'Unable to delete post'))
  }
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
