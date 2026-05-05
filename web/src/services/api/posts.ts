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

export function postToPayload(post: Post): PostPayload {
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
  }
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
