import { Button } from '@/components/ui/button'
import { useUpdateCampaign } from '@/hooks/useCampaigns'
import type { Campaign, CampaignStatus, UpdateCampaignPayload } from '@/types/campaigns'
import { cn } from '@/lib'

const STATUS_LABEL: Record<CampaignStatus, string> = {
  draft: 'Draft',
  scheduled: 'Scheduled',
  active: 'Active',
  paused: 'Paused',
  completed: 'Completed',
  archived: 'Archived',
}

const STATUS_COLOR: Record<CampaignStatus, string> = {
  draft: 'text-tertiary-foreground',
  scheduled: 'text-primary-foreground',
  active: 'text-positive',
  paused: 'text-tertiary-foreground',
  completed: 'text-primary-foreground',
  archived: 'text-tertiary-foreground',
}

function toPayload(campaign: Campaign, overrides: Partial<UpdateCampaignPayload>): UpdateCampaignPayload {
  return {
    name: campaign.name,
    campaign_type_id: campaign.campaign_type_id,
    description: campaign.description,
    target_persona: campaign.target_persona,
    key_messages: campaign.key_messages,
    tone_guidelines: campaign.tone_guidelines,
    use_assets: campaign.use_assets,
    asset_ids: campaign.asset_ids,
    target_platforms: campaign.target_platforms,
    status: campaign.status,
    start_date: campaign.start_date,
    end_date: campaign.end_date,
    estimated_post_count: campaign.estimated_post_count,
    budget: campaign.budget,
    currency: campaign.currency,
    language: campaign.language,
    tag_ids: campaign.tag_ids,
    ...overrides,
  }
}

export function CampaignHeaderActions({ campaign }: { campaign: Campaign }) {
  const { mutate: update, isPending } = useUpdateCampaign()

  const setStatus = (status: CampaignStatus) => {
    update({ id: campaign.id, payload: toPayload(campaign, { status }) })
  }

  const startInFuture =
    campaign.start_date != null && new Date(campaign.start_date).getTime() > Date.now()

  const primary = renderPrimary(campaign.status, startInFuture, setStatus, isPending)

  return (
    <div className="flex items-center gap-3">
      <span
        className={cn(
          'text-[13px] font-medium leading-10 uppercase tracking-wide',
          STATUS_COLOR[campaign.status],
        )}
      >
        {STATUS_LABEL[campaign.status]}
      </span>
      {primary}
    </div>
  )
}

function renderPrimary(
  status: CampaignStatus,
  startInFuture: boolean,
  setStatus: (s: CampaignStatus) => void,
  busy: boolean,
): React.ReactNode {
  switch (status) {
    case 'draft':
      return startInFuture ? (
        <Button variant="defaultInverted" onClick={() => setStatus('scheduled')} loading={busy}>
          SCHEDULE
        </Button>
      ) : (
        <Button variant="defaultInverted" onClick={() => setStatus('active')} loading={busy}>
          ACTIVATE
        </Button>
      )
    case 'scheduled':
      return (
        <Button variant="defaultInverted" onClick={() => setStatus('draft')} loading={busy}>
          UNPUBLISH
        </Button>
      )
    case 'active':
      return (
        <Button variant="defaultInverted" onClick={() => setStatus('paused')} loading={busy}>
          PAUSE
        </Button>
      )
    case 'paused':
      return (
        <Button variant="defaultInverted" onClick={() => setStatus('active')} loading={busy}>
          ACTIVATE
        </Button>
      )
    case 'completed':
      return (
        <Button variant="defaultInverted" onClick={() => setStatus('archived')} loading={busy}>
          ARCHIVE
        </Button>
      )
    case 'archived':
      return (
        <Button variant="defaultInverted" onClick={() => {/* TODO: duplicate */}} loading={busy}>
          DUPLICATE
        </Button>
      )
  }
}
