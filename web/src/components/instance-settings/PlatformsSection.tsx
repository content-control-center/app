import { memo, useState } from 'react'
import type { Platform } from '@/types/campaigns'
import { Icon } from '@/components/ui/icon'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib'

type PlatformsSectionProps = {
  platforms: Platform[]
}

function PlatformsSectionComponent({ platforms }: PlatformsSectionProps) {
  return (
    <section className="bg-primary">
      <div className="px-6 py-4">
        <h2 className="text-xl font-display font-medium tracking-tight">Platforms</h2>
      </div>
      {platforms.length === 0 ? (
        <div className="px-6 py-8 text-sm text-tertiary-foreground">No platforms.</div>
      ) : (
        <ul>
          {platforms.map((p) => (
            <PlatformRow key={p.id} platform={p} />
          ))}
        </ul>
      )}
    </section>
  )
}

function PlatformRow({ platform }: { platform: Platform }) {
  const [open, setOpen] = useState(true)
  const postTypeEntries = Object.entries(platform.post_types ?? {})

  return (
    <li>
      <Button
        type="button"
        variant="ghost"
        size="excluded"
        onClick={() => setOpen((v) => !v)}
        className="w-full justify-between px-6 py-4 rounded-none hover:bg-secondary/60"
        aria-expanded={open}
      >
        <span className="text-base font-medium">{platform.name || 'Untitled'}</span>
        <Icon
          name={open ? 'chevron_up' : 'chevron_down'}
          className="size-4 stroke-[2.5] text-tertiary-foreground"
        />
      </Button>
      <div
        className={cn(
          'overflow-hidden transition-[max-height] duration-200',
          open ? 'max-h-[1000px]' : 'max-h-0',
        )}
      >
        <div className="px-6 pb-5 pt-1 grid grid-cols-2 gap-x-8 text-sm">
          <dl className="grid grid-cols-[140px_1fr] gap-x-6 gap-y-3 content-start">
            <Field
              label="Cadence"
              value={
                platform.cadence?.trim()
                  ? <span className="whitespace-pre-wrap">{platform.cadence}</span>
                  : '—'
              }
            />
            <Field
              label="Constraints"
              value={
                platform.constraints?.trim()
                  ? <span className="whitespace-pre-wrap">{platform.constraints}</span>
                  : '—'
              }
            />
          </dl>
          <div>
            <div className="text-tertiary-foreground mb-3">Post types</div>
            {postTypeEntries.length === 0 ? (
              <div>—</div>
            ) : (
              <ul className="flex flex-col gap-2">
                {postTypeEntries.map(([slug, label]) => (
                  <li key={slug} className="flex items-center justify-between gap-3">
                    <span>{label}</span>
                    <Switch
                      checked={false}
                      onCheckedChange={() => {}}
                      disabled
                      aria-label={`Enable ${label}`}
                    />
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </div>
    </li>
  )
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <>
      <dt className="text-tertiary-foreground">{label}</dt>
      <dd className="min-w-0">{value}</dd>
    </>
  )
}

export const PlatformsSection = memo(PlatformsSectionComponent)
