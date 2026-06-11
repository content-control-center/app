import { useMemo } from 'react'

import { Plus, X } from '@phosphor-icons/react'
import { RailPanel } from '@/components/page-primitives/RailPanel'
import { AssetSection } from '../shared/AssetSection'
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
        actionIcon={X}
        actionAriaLabel={(a) => `Remove ${a.title || 'Untitled'}`}
        onAction={(a) => removeAsset(a.id)}
        defaultOpen
      />
      <AssetSection
        title="AVAILABLE IN CAMPAIGN"
        assets={availableInCampaign}
        emptyLabel="No campaign shortlist assets"
        actionIcon={Plus}
        actionAriaLabel={(a) => `Add ${a.title || 'Untitled'}`}
        onAction={(a) => addAsset(a.id)}
      />
      <AssetSection
        title="AVAILABLE"
        assets={available}
        emptyLabel="No other assets"
        actionIcon={Plus}
        actionAriaLabel={(a) => `Add ${a.title || 'Untitled'}`}
        onAction={(a) => addAsset(a.id)}
      />
    </RailPanel>
  )
}
