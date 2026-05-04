import type { CampaignStatus } from '@/types/campaigns'
import { StatusBadge, type StatusTone } from '@/components/ui/status-badge'

const CAMPAIGN_STATUS_LABEL: Record<CampaignStatus, string> = {
  draft: 'Draft',
  active: 'Active',
}

const CAMPAIGN_STATUS_TONE: Record<CampaignStatus, StatusTone> = {
  draft: 'neutral',
  active: 'positive',
}

type Props = {
  status: CampaignStatus
  className?: string
}

export function CampaignStatusBadge({ status, className }: Props) {
  return (
    <StatusBadge
      label={CAMPAIGN_STATUS_LABEL[status] ?? status}
      tone={CAMPAIGN_STATUS_TONE[status]}
      className={className}
    />
  )
}
