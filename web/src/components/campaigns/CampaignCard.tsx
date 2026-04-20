import type { Campaign } from '@/types/campaigns'

type CampaignCardProps = {
  campaign: Campaign
  onClick?: () => void
}

function formatDate(value: string | null): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

export function CampaignCard({ campaign, onClick }: CampaignCardProps) {
  const title = campaign.name.trim() || 'Untitled campaign'
  const typeLabel =
    campaign.campaign_type?.label ?? campaign.campaign_type?.name ?? null

  return (
    <div
      className="rounded-md bg-primary px-4 py-3 cursor-pointer hover:bg-secondary flex flex-col gap-2"
      onClick={onClick}
    >
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="font-medium truncate">{title}</div>
          {typeLabel && (
            <div className="text-xs text-tertiary-foreground mt-0.5">
              {typeLabel}
            </div>
          )}
        </div>
        <div className="text-xs text-tertiary-foreground shrink-0">
          {campaign.status}
        </div>
      </div>

      <div className="text-xs text-secondary-foreground">
        {formatDate(campaign.start_date)} — {formatDate(campaign.end_date)}
      </div>

      {campaign.platforms.length > 0 && (
        <div className="flex flex-wrap gap-1 text-xs">
          {campaign.platforms.map((platform) => (
            <span
              key={platform.id}
              className="px-2 py-0.5 rounded-sm bg-secondary text-secondary-foreground"
            >
              {platform.name}
            </span>
          ))}
        </div>
      )}

      {campaign.tags.length > 0 && (
        <div className="flex flex-wrap gap-1 text-xs">
          {campaign.tags.map((tag) => (
            <span
              key={tag.id}
              className="px-2 py-0.5 rounded-sm border border-border"
              style={{
                backgroundColor: tag.color,
                color: 'var(--color-foreground)',
              }}
            >
              {tag.name}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
