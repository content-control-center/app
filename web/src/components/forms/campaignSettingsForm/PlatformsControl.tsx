import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { CaretDownIcon, PlusIcon, XIcon } from '@phosphor-icons/react'
import { cn } from '@/lib'
import { usePlatformViews } from '@/hooks/usePlatforms'
import type { PlatformView } from '@/lib/platformDictionary'
import type { CampaignPlatform } from '@/types/campaigns'

type Props = {
  value: CampaignPlatform[]
  onChange: (next: CampaignPlatform[]) => void
}

function addPlatform(
  current: CampaignPlatform[],
  platformId: string,
  defaultSlugs: string[],
): CampaignPlatform[] {
  if (current.some((p) => p.id === platformId)) return current
  return [...current, { id: platformId, post_types: defaultSlugs }]
}

function removePlatform(
  current: CampaignPlatform[],
  platformId: string,
): CampaignPlatform[] {
  return current.filter((p) => p.id !== platformId)
}

function togglePostType(
  current: CampaignPlatform[],
  platformId: string,
  slug: string,
): CampaignPlatform[] {
  return current.map((p) => {
    if (p.id !== platformId) return p
    const has = p.post_types.includes(slug)
    return {
      ...p,
      post_types: has ? p.post_types.filter((s) => s !== slug) : [...p.post_types, slug],
    }
  })
}

export function PlatformsControl({ value, onChange }: Props) {
  const views = usePlatformViews()
  const selectedById = useMemo(
    () => new Map(value.map((p) => [p.id, p])),
    [value],
  )

  if (views.length === 0) {
    return (
      <span className="text-xs text-tertiary-foreground">No platforms available</span>
    )
  }

  return (
    <div className="flex flex-col gap-1">
      {views.map((view) => {
        const selected = selectedById.get(view.platform.id)
        return selected ? (
          <SelectedPlatformBlock
            key={view.platform.id}
            view={view}
            selectedPostTypes={selected.post_types}
            onTogglePostType={(slug) =>
              onChange(togglePostType(value, view.platform.id, slug))
            }
            onUnselect={() => onChange(removePlatform(value, view.platform.id))}
          />
        ) : (
          <UnselectedPlatformRow
            key={view.platform.id}
            view={view}
            onAdd={() =>
              onChange(
                addPlatform(
                  value,
                  view.platform.id,
                  view.available.map((pt) => pt.slug),
                ),
              )
            }
          />
        )
      })}
    </div>
  )
}

function UnselectedPlatformRow({ view, onAdd }: { view: PlatformView; onAdd: () => void }) {
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2.5 bg-secondary">
      <PlatformLabel view={view} />
      <Button
        type="button"
        variant="ghost"
        size="smIcon"
        onClick={onAdd}
        aria-label={`Add ${view.info.name}`}
        title={`Add ${view.info.name}`}
      >
        <PlusIcon className="size-4" />
      </Button>
    </div>
  )
}

function PlatformLabel({
  view,
  selectedCount,
}: {
  view: PlatformView
  selectedCount?: number
}) {
  const { info, available, connectedPublisherName } = view
  const isConnected = connectedPublisherName !== null
  return (
    <div className="min-w-0 flex flex-col">
      <span className="text-sm font-medium text-foreground">
        {info.name}
        {selectedCount !== undefined && selectedCount > 0  && (
          <span className="ml-1.5 text-tertiary-foreground font-normal">
            {selectedCount}
          </span>
        )}
      </span>
      <span className="text-xs text-tertiary-foreground truncate">
        {isConnected ? (
          <>
            {available.length} post types available via {connectedPublisherName}
          </>
        ) : (
          <>
            Not connected.{' '}
            <Link
              to="/instance-settings"
              className="text-primary-foreground hover:underline"
            >
              Open settings
            </Link>
          </>
        )}
      </span>
    </div>
  )
}

type SelectedBlockProps = {
  view: PlatformView
  selectedPostTypes: string[]
  onTogglePostType: (slug: string) => void
  onUnselect: () => void
}

function SelectedPlatformBlock({
  view,
  selectedPostTypes,
  onTogglePostType,
  onUnselect,
}: SelectedBlockProps) {
  const [open, setOpen] = useState(true)
  const { info, available, unavailable } = view

  const toggleOpen = () => setOpen((o) => !o)

  return (
    <div className="flex flex-col bg-secondary">
      <div className="flex items-center gap-1 px-3 py-2.5">
        <button
          type="button"
          onClick={toggleOpen}
          aria-expanded={open}
          className="flex-1 min-w-0 flex items-center text-left cursor-pointer"
        >
          <PlatformLabel view={view} selectedCount={selectedPostTypes.length} />
        </button>
        <Button
          type="button"
          variant="ghost"
          size="smIcon"
          onClick={onUnselect}
          aria-label={`Unselect ${info.name}`}
          title={`Unselect ${info.name}`}
        >
          <XIcon className="size-4" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="smIcon"
          onClick={toggleOpen}
          aria-label={open ? 'Collapse' : 'Expand'}
          aria-expanded={open}
        >
          <CaretDownIcon
            className={cn(
              'size-4 text-secondary-foreground transition-transform',
              open && 'rotate-180',
            )}
          />
        </Button>
      </div>
      <div
        className={cn(
          'grid transition-[grid-template-rows] duration-200 ease-out',
          open ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]',
        )}
      >
        <div className="overflow-hidden">
          <div className="flex flex-col pb-2">
            {available.map((pt) => (
              <PostTypeSwitchRow
                key={pt.slug}
                label={pt.label}
                ariaLabel={`${info.name} — ${pt.label}`}
                checked={selectedPostTypes.includes(pt.slug)}
                onToggle={() => onTogglePostType(pt.slug)}
              />
            ))}
            {unavailable.map((pt) => (
              <PostTypeSwitchRow
                key={pt.slug}
                label={pt.label}
                ariaLabel={`${info.name} — ${pt.label}`}
                checked={selectedPostTypes.includes(pt.slug)}
                onToggle={() => onTogglePostType(pt.slug)}
                muted
              />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

type PostTypeSwitchRowProps = {
  label: string
  ariaLabel: string
  checked: boolean
  onToggle: () => void
  muted?: boolean
}

function PostTypeSwitchRow({
  label,
  ariaLabel,
  checked,
  onToggle,
  muted = false,
}: PostTypeSwitchRowProps) {
  return (
    <label
      className={cn(
        'flex items-center justify-between gap-3 px-3 py-2 cursor-pointer select-none hover:bg-secondary/60',
        muted && 'opacity-60',
      )}
    >
      <span className="text-sm text-foreground">{label}</span>
      <Switch
        checked={checked}
        onCheckedChange={onToggle}
        aria-label={ariaLabel}
      />
    </label>
  )
}
