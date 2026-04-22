import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'

import { Icon } from '@/components/ui/icon'
import { RailPanel } from '@/components/page-primitives/RailPanel'
import { cn } from '@/lib'
import type { Post } from '@/types/posts'
import type { Asset } from '@/types/content'
import { useAssets } from '@/hooks/useContent'
import { useCampaign } from '@/hooks/useCampaigns'
import { usePostAutosave } from '../postSettingsForm/shared'

const schema = z.object({
  used_asset_ids: z.array(z.string()),
})

type FormValues = z.infer<typeof schema>

function defaultValues(post: Post): FormValues {
  return { used_asset_ids: post.used_asset_ids ?? [] }
}

type Props = {
  post: Post
  onClose?: () => void
}

export function PostContentUsageForm({ post, onClose }: Props) {
  const form = useForm<FormValues>({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    resolver: zodResolver(schema as any),
    defaultValues: defaultValues(post),
  })

  const assetIds = form.watch('used_asset_ids')
  const { data: assets } = useAssets()
  const { data: campaign } = useCampaign(post.campaign_id)

  usePostAutosave({
    post,
    form,
    buildOverrides: (v) => ({ used_asset_ids: v.used_asset_ids }),
  })

  const { selected, available } = useMemo(() => {
    const all = assets ?? []
    const campaignScope = new Set(campaign?.asset_ids ?? [])
    const inScope = campaignScope.size > 0 ? all.filter((a) => campaignScope.has(a.id)) : all
    const selectedSet = new Set(assetIds)
    const byId = new Map(inScope.map((a) => [a.id, a]))
    const selected: Asset[] = []
    for (const id of assetIds) {
      const a = byId.get(id)
      if (a) selected.push(a)
    }
    const available = inScope.filter((a) => !selectedSet.has(a.id))
    return { selected, available }
  }, [assets, assetIds, campaign])

  const addAsset = (id: string) => {
    if (assetIds.includes(id)) return
    form.setValue('used_asset_ids', [...assetIds, id], { shouldDirty: true })
  }

  const removeAsset = (id: string) => {
    form.setValue(
      'used_asset_ids',
      assetIds.filter((x) => x !== id),
      { shouldDirty: true },
    )
  }

  return (
    <RailPanel title="Content pieces" onClose={onClose}>
      <AssetSection
        title="Used"
        assets={selected}
        emptyLabel="No assets used"
        actionIcon="x_mark"
        actionAriaLabel={(a) => `Remove ${a.title || 'Untitled'}`}
        onAction={(a) => removeAsset(a.id)}
        defaultOpen
      />
      <AssetSection
        title="Available"
        assets={available}
        emptyLabel="No assets available"
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
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="flex flex-col">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex items-center justify-between gap-3 py-2 text-left cursor-pointer"
      >
        <span className="text-[13px] font-medium text-foreground">
          {title}
          <span className="ml-2 text-tertiary-foreground font-normal">
            {assets.length}
          </span>
        </span>
        <Icon
          name="chevron_down"
          className={cn(
            'size-4 shrink-0 text-secondary-foreground transition-transform',
            open && 'rotate-180',
          )}
        />
      </button>
      {open && (
        <div className="flex flex-col">
          {assets.length === 0 ? (
            <span className="text-xs text-tertiary-foreground py-2">{emptyLabel}</span>
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
        </div>
      )}
    </div>
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
    <div className="flex items-center gap-3 py-2">
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
