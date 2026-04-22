import type { Campaign } from '@/types/campaigns'
import { useCampaignPosts } from '@/hooks/usePosts'

type CampaignCardProps = {
  campaign: Campaign
  onClick?: () => void
}

const STATUS_DOT: Record<Campaign['status'], string> = {
  draft: 'bg-tertiary-foreground',
  active: 'bg-positive',
}

const STATUS_LABEL: Record<Campaign['status'], string> = {
  draft: 'Draft',
  active: 'Active',
}

export function CampaignCard({ campaign, onClick }: CampaignCardProps) {
  const title = campaign.name.trim() || 'Untitled campaign'
  const typeLabel =
    campaign.campaign_type?.label ?? campaign.campaign_type?.name ?? null

  const { data: posts } = useCampaignPosts(campaign.id)
  const totalPosts = posts?.length ?? 0
  const unpublishedPosts =
    posts?.filter((p) => p.status !== 'published').length ?? 0

  const platformCount =
    campaign.target_platforms?.length ?? campaign.platforms?.length ?? 0

  return (
    <div
      className="rounded-md bg-primary px-4 py-4 cursor-pointer hover:bg-secondary flex flex-col gap-3 h-full"
      onClick={onClick}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="font-medium truncate">{title}</div>
          {typeLabel && (
            <div className="text-xs text-tertiary-foreground mt-0.5">
              {typeLabel}
            </div>
          )}
        </div>
        <div className="flex items-center gap-1.5 text-xs text-tertiary-foreground shrink-0">
          <span
            className={`size-2 rounded-full ${STATUS_DOT[campaign.status]}`}
          />
          <span>{STATUS_LABEL[campaign.status] ?? campaign.status}</span>
        </div>
      </div>

      <div className="flex items-center gap-4 text-xs text-secondary-foreground">
        <div>
          <span className="font-medium text-foreground">
            {unpublishedPosts}
          </span>
          <span className="text-tertiary-foreground"> / {totalPosts}</span>
          <span className="ml-1">unpublished</span>
        </div>
        <div>
          <span className="font-medium text-foreground">{platformCount}</span>
          <span className="ml-1">
            {platformCount === 1 ? 'platform' : 'platforms'}
          </span>
        </div>
      </div>

      {campaign.tags.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {campaign.tags.map((tag) => (
            <span
              key={tag.id}
              className="inline-flex items-center rounded-full bg-quaternary text-primary-foreground text-[12px] font-medium px-2.5 py-0.5"
            >
              {tag.name}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
