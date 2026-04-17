import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'
import type { Campaign } from '@/types/campaigns'

type Props = {
  campaign: Campaign
  onOpen: () => void
}

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

export function CampaignSettingsButton({ campaign, onOpen }: Props) {
  const complete = isBriefComplete(campaign)

  if (complete) {
    return (
      <Button
        variant="default"
        size="defaultIcon"
        aria-label="Settings"
        onClick={onOpen}
      >
        <Icon name="settings" className="size-4" />
      </Button>
    )
  }

  return (
    <Button variant="defaultInverted" onClick={onOpen} aria-label="Settings">
      <Icon name="settings" className="size-4" />
      <span>SETTINGS</span>
    </Button>
  )
}
