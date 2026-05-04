import { Button } from '@/components/ui/button'
import { useUpdateCampaign } from '@/hooks/useCampaigns'
import type { Campaign, CampaignStatus, UpdateCampaignPayload } from '@/types/campaigns'
import { CampaignStatusBadge } from '@/components/campaigns/CampaignStatusBadge'

function isBriefComplete(campaign: Campaign): boolean {
  const required = [
    campaign.name,
    campaign.campaign_type_id,
    campaign.description,
    campaign.target_persona,
    campaign.key_messages,
    campaign.tone_guidelines,
  ]
  return required.every((v) => v != null && v.trim() !== '')
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

  const settingsComplete = isBriefComplete(campaign)

  return (
    <div className="flex items-center gap-3">
      <CampaignStatusBadge status={campaign.status} />
      {campaign.status === 'draft' ? (
        <Button
          variant={settingsComplete ? "defaultInverted" : "default" }
          onClick={() => setStatus('active')}
          loading={isPending}
        >
          ACTIVATE
        </Button>
      ) : (
        <Button
          variant="default"
          onClick={() => setStatus('draft')}
          loading={isPending}
        >
          DEACTIVATE
        </Button>
      )}
    </div>
  )
}
