import { useMemo } from 'react'
import * as SelectPrimitive from '@radix-ui/react-select'

import { CaretDownIcon } from '@phosphor-icons/react'
import { cn } from '@/lib'
import { ZIndex } from '@/config/zIndex'
import { selectTriggerVariants } from '@/components/ui/text-select'
import { usePlatformViews } from '@/hooks/usePlatforms'
import type { Campaign } from '@/types/campaigns'

type Item = {
  id: string
  platformId: string
  slug: string
  label: string
}

function joinId(platformId: string, slug: string): string {
  return `${platformId}::${slug}`
}

function splitId(id: string): { platformId: string; slug: string } {
  const idx = id.indexOf('::')
  if (idx < 0) return { platformId: id, slug: '' }
  return { platformId: id.slice(0, idx), slug: id.slice(idx + 2) }
}

type Props = {
  campaign: Campaign | undefined
  platformId: string
  postType: string
  onChange: (platformId: string, postType: string) => void
  placeholder?: string
  disabled?: boolean
}

export function CampaignPostTypeSelect({
  campaign,
  platformId,
  postType,
  onChange,
  placeholder = 'Select post type',
  disabled,
}: Props) {
  const views = usePlatformViews()

  const { active, available, otherCount, currentLabel, reasonNoActive } =
    useMemo(() => {
      const targetPlatforms = campaign?.target_platforms ?? []
      const campaignMap = new Map(
        targetPlatforms.map((p) => [p.id, new Set(p.post_types)]),
      )

      const active: Item[] = []
      const available: Item[] = []
      let otherCount = 0
      let currentLabel: string | null = null
      let anySelected = false

      for (const view of views) {
        const availableSlugs = new Set(view.available.map((pt) => pt.slug))
        const camp = campaignMap.get(view.platform.id)
        if (camp && camp.size > 0) anySelected = true

        for (const pt of view.info.postTypes) {
          const id = joinId(view.platform.id, pt.slug)
          const item: Item = {
            id,
            platformId: view.platform.id,
            slug: pt.slug,
            label: `${view.info.name} - ${pt.label}`,
          }
          if (id === joinId(platformId, postType)) currentLabel = item.label

          const onCampaign = camp?.has(pt.slug) ?? false
          const isAvailable = availableSlugs.has(pt.slug)
          if (onCampaign && isAvailable) active.push(item)
          else if (onCampaign) available.push(item)
          else otherCount += 1
        }
      }

      let reasonNoActive: string | null = null
      if (active.length === 0) {
        if (targetPlatforms.length === 0) {
          reasonNoActive = 'No platforms added to this campaign yet.'
        } else if (!anySelected) {
          reasonNoActive = 'No post types selected on this campaign yet.'
        } else {
          reasonNoActive =
            'Selected post types are not set up — connect a publisher in settings.'
        }
      }

      return { active, available, otherCount, currentLabel, reasonNoActive }
    }, [views, campaign, platformId, postType])

  const value = platformId && postType ? joinId(platformId, postType) : ''

  return (
    <SelectPrimitive.Root
      value={value || undefined}
      disabled={disabled}
      onValueChange={(next) => {
        const { platformId: pid, slug } = splitId(next)
        onChange(pid, slug)
      }}
    >
      <SelectPrimitive.Trigger
        className={cn(
          selectTriggerVariants({ variant: 'default', size: 'default' }),
          'justify-between gap-3 pr-2 cursor-pointer',
        )}
      >
        <SelectPrimitive.Value placeholder={placeholder}>
          {currentLabel ?? placeholder}
        </SelectPrimitive.Value>
        <div className="flex h-8 w-8 items-center justify-center">
          <CaretDownIcon className="size-4" />
        </div>
      </SelectPrimitive.Trigger>

      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          position="popper"
          style={{ zIndex: ZIndex.popover }}
          className={cn(
            'bg-popover text-popover-foreground rounded-sm shadow-lg overflow-hidden',
            'min-w-[var(--radix-select-trigger-width)] max-h-[var(--radix-select-content-available-height)]',
            'data-[state=open]:animate-in data-[state=closed]:animate-out',
            'data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
          )}
        >
          <SelectPrimitive.Viewport className="p-1">
            <SelectPrimitive.Group>
              <GroupLabel>Active</GroupLabel>
              {active.length > 0 ? (
                active.map((it) => <GroupItem key={it.id} item={it} />)
              ) : (
                <InfoRow>{reasonNoActive}</InfoRow>
              )}
            </SelectPrimitive.Group>

            {available.length > 0 && (
              <>
                <Divider />
                <SelectPrimitive.Group>
                  <GroupLabel>Available</GroupLabel>
                  {available.map((it) => (
                    <GroupItem key={it.id} item={it} />
                  ))}
                </SelectPrimitive.Group>
              </>
            )}

            {otherCount > 0 && (
              <>
                <Divider />
                <InfoRow>
                  {otherCount} other post type{otherCount === 1 ? '' : 's'} not
                  added to this campaign
                </InfoRow>
              </>
            )}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  )
}

function GroupLabel({ children }: { children: React.ReactNode }) {
  return (
    <SelectPrimitive.Label className="px-2 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-wider text-tertiary-foreground">
      {children}
    </SelectPrimitive.Label>
  )
}

function GroupItem({ item }: { item: Item }) {
  return (
    <SelectPrimitive.Item
      value={item.id}
      className={cn(
        'relative flex w-full cursor-pointer items-center rounded-sm py-1.5 px-2 text-sm outline-none',
        'focus:bg-primary focus:text-primary-foreground',
        'data-[state=checked]:font-medium',
      )}
    >
      <SelectPrimitive.ItemText>{item.label}</SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  )
}

function InfoRow({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-2 py-1.5 text-xs text-tertiary-foreground">{children}</div>
  )
}

function Divider() {
  return <SelectPrimitive.Separator className="my-1 h-px bg-border" />
}
