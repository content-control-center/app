import { useMemo } from 'react'

import { Icon } from '@/components/ui/icon'
import { Collapse } from '@/components/ui/collapse'
import { RailPanel } from '@/components/page-primitives/RailPanel'
import type { Asset } from '@/types/content'
import type { Post } from '@/types/posts'
import { useAssets } from '@/hooks/useContent'
import { useCampaign, useUpdateCampaign } from '@/hooks/useCampaigns'
import { campaignToPayload } from '../campaignBriefForm/shared'

type Props = {
  doc: Post
  changeDoc: (fn: (p: Post) => void) => void
  onClose?: () => void
}

export function PostContentUsageForm({ doc, changeDoc, onClose }: Props) {
  const assetIds = doc.used_asset_ids
  const { data: assets } = useAssets()
  const { data: campaign } = useCampaign(doc.campaign_id)
  const { mutate: updateCampaign } = useUpdateCampaign()

  const { selected, availableInCampaign, available } = useMemo(() => {
    const all = assets ?? []
    const selectedSet = new Set(assetIds)
    const campaignSet = new Set(campaign?.asset_ids ?? [])
    const byId = new Map(all.map((a) => [a.id, a]))

    const selected: Asset[] = []
    for (const id of assetIds) {
      const a = byId.get(id)
      if (a) selected.push(a)
    }

    const availableInCampaign: Asset[] = []
    const available: Asset[] = []
    for (const a of all) {
      if (selectedSet.has(a.id)) continue
      if (campaignSet.has(a.id)) availableInCampaign.push(a)
      else available.push(a)
    }
    return { selected, availableInCampaign, available }
  }, [assets, assetIds, campaign])

  const addAsset = (id: string) => {
    changeDoc((d) => {
      if (d.used_asset_ids.includes(id)) return
      d.used_asset_ids.push(id)
    })
    if (campaign && !campaign.asset_ids.includes(id)) {
      const nextIds = [...campaign.asset_ids, id]
      updateCampaign({
        id: campaign.id,
        payload: campaignToPayload(campaign, {
          asset_ids: nextIds,
          use_assets: true,
        }),
      })
    }
  }

  const removeAsset = (id: string) => {
    changeDoc((d) => {
      const idx = d.used_asset_ids.indexOf(id)
      if (idx >= 0) d.used_asset_ids.splice(idx, 1)
    })
  }

  return (
    <RailPanel title="Content pieces" onClose={onClose}>
      <AssetSection
        title="SELECTED"
        assets={selected}
        emptyLabel="No assets used"
        actionIcon="x_mark"
        actionAriaLabel={(a) => `Remove ${a.title || 'Untitled'}`}
        onAction={(a) => removeAsset(a.id)}
        defaultOpen
      />
      <AssetSection
        title="AVAILABLE IN CAMPAIGN"
        assets={availableInCampaign}
        emptyLabel="No campaign shortlist assets"
        actionIcon="plus"
        actionAriaLabel={(a) => `Add ${a.title || 'Untitled'}`}
        onAction={(a) => addAsset(a.id)}
      />
      <AssetSection
        title="AVAILABLE"
        assets={available}
        emptyLabel="No other assets"
        actionIcon="plus"
        actionAriaLabel={(a) => `Add ${a.title || 'Untitled'}`}
        onAction={(a) => addAsset(a.id)}
      />
    </RailPanel>
  )
}

type AssetSectionProps = {
  title: string
  assets: Asset[]
  emptyLabel: string
  actionIcon: 'plus' | 'x_mark'
  actionAriaLabel: (a: Asset) => string
  onAction: (a: Asset) => void
  defaultOpen?: boolean
}

function AssetSection({
  title,
  assets,
  emptyLabel,
  actionIcon,
  actionAriaLabel,
  onAction,
  defaultOpen = false,
}: AssetSectionProps) {
  return (
    <Collapse title={title} meta={assets.length} defaultOpen={defaultOpen}>
      {assets.length === 0 ? (
        <div className="flex items-center min-h-[52px] py-2">
          <span className="text-[14px] leading-4 text-tertiary-foreground">{emptyLabel}</span>
        </div>
      ) : (
        assets.map((a) => (
          <AssetRow
            key={a.id}
            asset={a}
            actionIcon={actionIcon}
            actionAriaLabel={actionAriaLabel(a)}
            onAction={() => onAction(a)}
          />
        ))
      )}
    </Collapse>
  )
}

type AssetRowProps = {
  asset: Asset
  actionIcon: 'plus' | 'x_mark'
  actionAriaLabel: string
  onAction: () => void
}

function AssetRow({ asset, actionIcon, actionAriaLabel, onAction }: AssetRowProps) {
  const title = asset.title.trim() === '' ? 'Untitled' : asset.title
  const type = asset.tags?.[0]?.name ?? 'Asset'
  return (
    <div className="flex items-center gap-3 min-h-[52px] py-2">
      <div className="min-w-0 flex-1 flex flex-col">
        <span className="text-[13px] text-foreground truncate">{title}</span>
        <span className="text-xs text-tertiary-foreground truncate">{type}</span>
      </div>
      <button
        type="button"
        onClick={onAction}
        aria-label={actionAriaLabel}
        className="flex items-center justify-center size-6 shrink-0 text-secondary-foreground hover:text-foreground cursor-pointer"
      >
        <Icon name={actionIcon} className="size-4" />
      </button>
    </div>
  )
}
