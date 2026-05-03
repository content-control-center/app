import { memo, useId, useState, type ReactNode } from 'react'
import type { Platform, PlatformPublisher, PublisherAccount } from '@/types/campaigns'
import { Icon } from '@/components/ui/icon'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Chip } from '@/components/ui/chip'
import { ModalContainer } from '@/components/ui/modal'
import { getPlatformInfo, type PlatformPostType } from '@/lib/platformDictionary'
import { cn } from '@/lib'
import { SettingsRow } from './SettingsRow'

type PlatformsSectionProps = {
  platforms: Platform[]
}

function PlatformsSectionComponent({ platforms }: PlatformsSectionProps) {
  const known = platforms.filter((p) => getPlatformInfo(p.id))

  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-xl font-display font-medium tracking-tight">Platforms</h2>
      {known.length === 0 ? (
        <div className="bg-primary px-6 py-5 text-sm text-tertiary-foreground">No platforms.</div>
      ) : (
        <ul className="flex flex-col gap-4">
          {known.map((p) => (
            <PlatformRow key={p.id} platform={p} />
          ))}
        </ul>
      )}
    </section>
  )
}

function PlatformRow({ platform }: { platform: Platform }) {
  const info = getPlatformInfo(platform.id)!
  const publishers = platform.publishers ?? []
  const anyConnected = publishers.some((p) => p.connected)
  const accountPublishers = publishers.filter((p) => p.accounts.length > 0)
  const hasPublisher = publishers.length > 0

  return (
    <SettingsRow
      title={info.name}
      badges={<ConnectionPill connected={anyConnected} hasPublisher={hasPublisher} />}
      actions={hasPublisher ? <PlatformEditIconButton platform={platform} /> : null}
    >
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-x-8 gap-y-5">
        <PostTypeChips postTypes={info.postTypes} publishers={publishers} />
        <div className="flex flex-col gap-4">
          <ReadOnlyField label="Cadence" value={platform.cadence} />
          <ReadOnlyField label="Constraints" value={platform.constraints} />
        </div>
      </div>
      {accountPublishers.length > 0 && (
        <div className="flex flex-col gap-3">
          {accountPublishers.map((p) => (
            <PublisherAccounts key={p.id} publisher={p} />
          ))}
        </div>
      )}
    </SettingsRow>
  )
}

function PostTypeChips({
  postTypes,
  publishers,
}: {
  postTypes: PlatformPostType[]
  publishers: PlatformPublisher[]
}) {
  const groups = publishers.map((pub) => {
    const supported = new Set(pub.supported_post_types)
    const isZernio = pub.name.toLowerCase() === 'zernio'
    return {
      key: pub.id,
      label: `Available via ${pub.name}`,
      connected: pub.connected,
      items: postTypes.filter((pt) => supported.has(pt.slug)),
      emptyAction:
        !pub.connected && isZernio ? (
          <a
            href="https://zernio.com/dashboard/connections"
            target="_blank"
            rel="noopener noreferrer"
            className="text-primary-foreground hover:underline inline-flex items-center gap-1"
          >
            Connect on Zernio
            <Icon name="arrow_right_top" className="size-2.5 stroke-2" />
          </a>
        ) : null,
    }
  })
  const supportedAnywhere = new Set<string>()
  for (const p of publishers) for (const slug of p.supported_post_types) supportedAnywhere.add(slug)
  const unavailable = postTypes.filter((pt) => !supportedAnywhere.has(pt.slug))

  return (
    <div className="flex flex-col gap-4">
      {groups.length === 0 ? (
        <ChipGroup label="Available" items={[]} emptyText="No publisher integration configured." />
      ) : (
        groups.map((g) => (
          <ChipGroup
            key={g.key}
            label={g.label}
            items={g.connected ? g.items : []}
            emptyText={g.connected ? 'None' : 'No Post types available.'}
            emptyAction={g.emptyAction}
          />
        ))
      )}
      {unavailable.length > 0 && (
        <ChipGroup label="Unavailable" items={unavailable} emptyText="None" muted />
      )}
    </div>
  )
}

function ChipGroup({
  label,
  items,
  emptyText,
  emptyAction,
  muted = false,
}: {
  label: string
  items: PlatformPostType[]
  emptyText: string
  emptyAction?: ReactNode
  muted?: boolean
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[13px] font-normal text-input-label h-4">{label}</span>
      {items.length === 0 ? (
        <div className={'h-10 py-1 border-b border-transparent flex items-center gap-2 flex-wrap'}>
          <span className="text-[13px] leading-4 text-primary-foreground">{emptyText}</span>
          {emptyAction && (
            <span className="text-[13px] leading-4">{emptyAction}</span>
          )}
        </div>
      ) : (
        <ul className="flex flex-wrap gap-1.5">
          {items.map((pt) => (
            <li key={pt.slug}>
              <Chip variant={muted ? 'muted' : 'default'}>{pt.label}</Chip>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function ReadOnlyField({
  label,
  value,
}: {
  label: string
  value: string | undefined
}) {
  const text = value?.trim() ?? ''
  const id = useId()
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input id={id} value={text} readOnly disabled placeholder="—" title={text || undefined} />
    </div>
  )
}

function PlatformEditIconButton({ platform }: { platform: Platform }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="smIcon"
        onClick={() => setOpen(true)}
        aria-label="Edit platform"
        title="Edit platform"
      >
        <Icon name="edit" className="size-3.5" />
      </Button>
      <PlatformDetailsModal
        platform={platform}
        open={open}
        onClose={() => setOpen(false)}
      />
    </>
  )
}

function PlatformDetailsModal({
  platform,
  open,
  onClose,
}: {
  platform: Platform
  open: boolean
  onClose: () => void
}) {
  const info = getPlatformInfo(platform.id)
  const cadenceId = useId()
  const constraintsId = useId()
  return (
    <ModalContainer
      isOpen={open}
      onClose={onClose}
      title={`${info?.name ?? platform.name} settings`}
      size="default"
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={cadenceId}>Cadence</Label>
          <Textarea
            id={cadenceId}
            value={platform.cadence ?? ''}
            readOnly
            disabled
            placeholder="—"
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={constraintsId}>Constraints</Label>
          <Textarea
            id={constraintsId}
            value={platform.constraints ?? ''}
            readOnly
            disabled
            placeholder="—"
          />
        </div>
      </div>
    </ModalContainer>
  )
}

function PublisherAccounts({ publisher }: { publisher: PlatformPublisher }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[13px] font-normal text-input-label">
        {publisher.name} accounts
      </span>
      <ul className="flex flex-col gap-1.5">
        {publisher.accounts.map((a) => (
          <AccountRow key={a.id} account={a} />
        ))}
      </ul>
    </div>
  )
}

function AccountRow({ account }: { account: PublisherAccount }) {
  const initial = (account.display_name || account.username || '?').slice(0, 1).toUpperCase()
  return (
    <li className="flex items-center gap-2 text-sm">
      <Avatar className="size-6">
        {account.avatar_url && <AvatarImage src={account.avatar_url} alt={account.username} />}
        <AvatarFallback className="text-[10px]">{initial}</AvatarFallback>
      </Avatar>
      <span className="truncate">@{account.username}</span>
      {account.display_name && (
        <span className="text-tertiary-foreground truncate">· {account.display_name}</span>
      )}
      {!account.is_active && (
        <span className="text-xs text-tertiary-foreground">(inactive)</span>
      )}
    </li>
  )
}

function ConnectionPill({
  connected,
  hasPublisher,
}: {
  connected: boolean
  hasPublisher: boolean
}) {
  if (!hasPublisher) return null
  return (
    <span
      className={cn(
        'text-[11px] leading-4 px-1.5 py-[1px] rounded-md border',
        connected
          ? 'bg-transparent text-primary-foreground border-primary-foreground'
          : 'bg-tertiary text-tertiary-foreground border-transparent',
      )}
    >
      {connected ? 'Connected' : 'Not connected'}
    </span>
  )
}

export const PlatformsSection = memo(PlatformsSectionComponent)
