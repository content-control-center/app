import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useDeleteCampaign, useUpdateCampaign } from '@/hooks/useCampaigns'
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

type PrimaryVariant = 'default' | 'outline'

function toPayload(campaign: Campaign, overrides: Partial<UpdateCampaignPayload>): UpdateCampaignPayload {
  return {
    name: campaign.name,
    campaign_type_id: campaign.campaign_type_id,
    description: campaign.description,
    target_persona: campaign.target_persona,
    key_messages: campaign.key_messages,
    tone_guidelines: campaign.tone_guidelines,
    use_pieces: campaign.use_pieces,
    pieces_ids: campaign.pieces_ids,
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
  const { mutate: update, isPending: isUpdating } = useUpdateCampaign()
  const { mutate: remove, isPending: isDeleting } = useDeleteCampaign()

  const setStatus = (status: CampaignStatus) => {
    update({ id: campaign.id, payload: toPayload(campaign, { status }) })
  }

  const startInFuture =
    campaign.start_date != null && new Date(campaign.start_date).getTime() > Date.now()

  const busy = isUpdating || isDeleting

  const { node: primary, variant: primaryVariant } = renderPrimary(
    campaign.status,
    startInFuture,
    setStatus,
    busy,
  )

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
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant={primaryVariant}
            size="defaultIcon"
            aria-label="More actions"
            disabled={busy}
          >
            <Icon name="dots_2_vertical" className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuGroup>
            {campaign.status !== 'archived' && (
              <DropdownMenuItem onSelect={() => setStatus('archived')}>
                ARCHIVE
              </DropdownMenuItem>
            )}
            <DropdownMenuItem onSelect={() => {/* TODO: duplicate */}}>
              DUPLICATE
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => remove(campaign.id)}
            >
              DELETE
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function renderPrimary(
  status: CampaignStatus,
  startInFuture: boolean,
  setStatus: (s: CampaignStatus) => void,
  busy: boolean,
): { node: React.ReactNode; variant: PrimaryVariant } {
  switch (status) {
    case 'draft': {
      const variant: PrimaryVariant = 'default'
      return {
        variant,
        node: startInFuture ? (
          <Button variant={variant} onClick={() => setStatus('scheduled')} loading={busy}>
            SCHEDULE
          </Button>
        ) : (
          <Button variant={variant} onClick={() => setStatus('active')} loading={busy}>
            ACTIVATE
          </Button>
        ),
      }
    }
    case 'scheduled': {
      const variant: PrimaryVariant = 'outline'
      return {
        variant,
        node: (
          <Button variant={variant} onClick={() => setStatus('draft')} loading={busy}>
            UNPUBLISH
          </Button>
        ),
      }
    }
    case 'active': {
      const variant: PrimaryVariant = 'outline'
      return {
        variant,
        node: (
          <Button variant={variant} onClick={() => setStatus('paused')} loading={busy}>
            PAUSE
          </Button>
        ),
      }
    }
    case 'paused': {
      const variant: PrimaryVariant = 'default'
      return {
        variant,
        node: (
          <Button variant={variant} onClick={() => setStatus('active')} loading={busy}>
            ACTIVATE
          </Button>
        ),
      }
    }
    case 'completed': {
      const variant: PrimaryVariant = 'outline'
      return {
        variant,
        node: (
          <Button variant={variant} onClick={() => setStatus('archived')} loading={busy}>
            ARCHIVE
          </Button>
        ),
      }
    }
    case 'archived': {
      const variant: PrimaryVariant = 'outline'
      return {
        variant,
        node: (
          <Button variant={variant} onClick={() => {/* TODO: duplicate */}} loading={busy}>
            DUPLICATE
          </Button>
        ),
      }
    }
  }
}
