import type { Campaign } from '@/types/campaigns'

export type PostStatus =
  | 'draft'
  | 'ready_for_publish'
  | 'scheduled'
  | 'scheduled_for_manual_publishing'
  | 'failed'
  | 'published'
  | 'not_published'

export type PostCTAType = 'link' | 'button' | 'none'

export type Post = {
  id: string
  campaign_id: string
  platform_id: string
  platform_post_type: string
  title: string
  content: string
  media_urls: string[]
  scheduled_at: string | null
  published_at: string | null
  status: PostStatus
  cta_type: PostCTAType
  cta_url: string
  target_audience_notes: string
  used_asset_ids: string[]
  campaign_type_phase_id: string | null
  created_by: string
  created_at: string
  updated_at: string
  campaign: Campaign | null
  platform: { id: string; name: string; post_types: Record<string, string>; cadence: string; constraints: string; created_at: string; updated_at: string } | null
  used_assets: unknown[]
  campaign_type_phase: unknown | null
}

export const POST_STATUS_LABELS: Record<PostStatus, string> = {
  draft: 'Draft',
  ready_for_publish: 'Ready for Publish',
  scheduled: 'Auto-publish',
  scheduled_for_manual_publishing: 'Manual publish',
  failed: 'Failed',
  published: 'Published',
  not_published: 'Not Published',
}

export const DELETABLE_STATUSES: PostStatus[] = [
  'draft',
  'failed',
  'not_published',
]

export type PostPayload = {
  campaign_id: string
  platform_id?: string
  platform_post_type?: string
  title?: string
  content?: string
  media_urls?: string[]
  scheduled_at?: string | null
  published_at?: string | null
  status?: PostStatus
  cta_type?: PostCTAType
  cta_url?: string
  target_audience_notes?: string
  used_asset_ids?: string[]
  campaign_type_phase_id?: string | null
}
