import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'

import { Plus, X } from '@phosphor-icons/react'
import { RailPanel } from '@/components/page-primitives/RailPanel'
import { AssetSection } from '../shared/AssetSection'
import type { Campaign } from '@/types/campaigns'
import type { Asset } from '@/types/content'
import { useAssets } from '@/hooks/useContent'
import { useCampaignAutosave } from '../campaignBriefForm/shared'

const schema = z.object({
  asset_ids: z.array(z.string()),
})

type FormValues = z.infer<typeof schema>

function defaultValues(campaign: Campaign): FormValues {
  return {
    asset_ids: campaign.asset_ids ?? [],
  }
}

type Props = {
  campaign: Campaign
  onFlushRef?: (flush: () => void) => void
  onClose?: () => void
}

export function CampaignContentUsageForm({ campaign, onFlushRef, onClose }: Props) {
  const form = useForm<FormValues>({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    resolver: zodResolver(schema as any),
    defaultValues: defaultValues(campaign),
  })

  const assetIds = form.watch('asset_ids')
  const { data: assets } = useAssets()

  useCampaignAutosave({
    campaign,
    form,
    buildOverrides: (v) => ({
      asset_ids: v.asset_ids,
      use_assets: v.asset_ids.length > 0,
    }),
    onFlushRef,
  })

  const { selected, available } = useMemo(() => {
    const all = assets ?? []
    const selectedSet = new Set(assetIds)
    const selected: Asset[] = []
    const selectedById = new Map(all.map((a) => [a.id, a]))
    for (const id of assetIds) {
      const a = selectedById.get(id)
      if (a) selected.push(a)
    }
    const available = all.filter((a) => !selectedSet.has(a.id))
    return { selected, available }
  }, [assets, assetIds])

  const addAsset = (id: string) => {
    if (assetIds.includes(id)) return
    form.setValue('asset_ids', [...assetIds, id], { shouldDirty: true })
  }

  const removeAsset = (id: string) => {
    form.setValue('asset_ids', assetIds.filter((x) => x !== id), { shouldDirty: true })
  }

  return (
    <RailPanel title="Content Bank Assets" onClose={onClose}>
      <AssetSection
        title="SELECTED"
        assets={selected}
        emptyLabel="No assets selected"
        actionIcon={X}
        actionAriaLabel={(a) => `Remove ${a.title || 'Untitled'}`}
        onAction={(a) => removeAsset(a.id)}
        defaultOpen
      />
      <AssetSection
        title="AVAILABLE"
        assets={available}
        emptyLabel="No assets available"
        actionIcon={Plus}
        actionAriaLabel={(a) => `Add ${a.title || 'Untitled'}`}
        onAction={(a) => addAsset(a.id)}
      />
    </RailPanel>
  )
}
