import { memo, useEffect, useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { ModalContainer } from '@/components/ui/modal'
import { ArrowUpRightIcon, PencilSimpleIcon } from '@phosphor-icons/react'
import { useDeleteSecret, useSecretsList, useUpsertSecret } from '@/hooks/useSecrets'
import { useZernioHealth } from '@/hooks/useZernio'
import type { SecretMetadata, SecretName, ZernioState } from '@/types/integrations'
import { cn } from '@/lib'
import { SettingsRow } from './SettingsRow'

type SecretDef = {
  name: SecretName
  label: string
  description: string
  providerLabel: string
  providerUrl: string
  mask: string
  inputPlaceholder: string
}

const MASK_CHAR = '×'
const ANTHROPIC_BODY_LEN = 79
const ANTHROPIC_MASK = `sk-ant-api03-${MASK_CHAR.repeat(6)}-${MASK_CHAR.repeat(ANTHROPIC_BODY_LEN)}-${MASK_CHAR.repeat(8)}`
const ZERNIO_BODY_LEN = 64
const ZERNIO_MASK = `sk_${MASK_CHAR.repeat(ZERNIO_BODY_LEN)}`

const SECRETS: SecretDef[] = [
  {
    name: 'anthropic_api_key',
    label: 'Anthropic',
    description: 'Powers Claude-based content generation across the app.',
    providerLabel: 'console.anthropic.com',
    providerUrl: 'https://console.anthropic.com/settings/keys',
    mask: ANTHROPIC_MASK,
    inputPlaceholder: 'sk-ant-...',
  },
  {
    name: 'zernio_api_key',
    label: 'Zernio',
    description: 'Connects social platforms so posts can be published from here.',
    providerLabel: 'zernio.com',
    providerUrl: 'https://zernio.com/account/api',
    mask: ZERNIO_MASK,
    inputPlaceholder: 'sk_...',
  },
]

function ApiKeysSectionComponent() {
  const { data, isLoading, isError } = useSecretsList()

  const byName = new Map<SecretName, SecretMetadata>()
  for (const m of data ?? []) byName.set(m.name, m)

  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-xl font-display font-medium tracking-tight">Integrations</h2>
      {isError ? (
        <div className="bg-primary px-6 py-5 text-sm text-destructive">
          Failed to load integrations.
        </div>
      ) : (
        <ul className="flex flex-col gap-4">
          {SECRETS.map((s) => (
            <IntegrationRow
              key={s.name}
              def={s}
              metadata={byName.get(s.name) ?? null}
              loading={isLoading}
            />
          ))}
        </ul>
      )}
    </section>
  )
}

type IntegrationRowProps = {
  def: SecretDef
  metadata: SecretMetadata | null
  loading: boolean
}

function IntegrationRow({ def, metadata, loading }: IntegrationRowProps) {
  const [editOpen, setEditOpen] = useState(false)

  const configured = metadata !== null
  const isZernio = def.name === 'zernio_api_key'

  return (
    <SettingsRow
      title={def.label}
      badges={isZernio && configured ? <ZernioStatePill /> : null}
      actions={
        <Button
          type="button"
          variant="ghost"
          size="smIcon"
          onClick={() => setEditOpen(true)}
          disabled={loading}
          aria-label="Edit API key"
          title="Edit API key"
        >
          <PencilSimpleIcon className="size-3.5" />
        </Button>
      }
      description={
        <>
          <p>{def.description}</p>
          <a
            href={def.providerUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="text-primary-foreground inline-flex items-center gap-1 hover:underline"
          >
            Get an API key at {def.providerLabel}
            <ArrowUpRightIcon className="size-2.5 mt-1" />
          </a>
        </>
      }
    >
      <div className="flex flex-col gap-1.5 w-full">
        <Input
          readOnly
          disabled
          value={configured ? def.mask : ''}
          placeholder="Not added"
          className="font-mono w-full"
          aria-label={`${def.label} API key`}
        />
        <p className="text-xs text-tertiary-foreground">
          {renderHelper({ configured, metadata, isZernio })}
        </p>
      </div>

      <EditKeyModal
        open={editOpen}
        onClose={() => setEditOpen(false)}
        def={def}
        configured={configured}
      />
    </SettingsRow>
  )
}

function renderHelper({
  configured,
  metadata,
  isZernio,
}: {
  configured: boolean
  metadata: SecretMetadata | null
  isZernio: boolean
}) {
  if (!configured) return 'Not configured'
  return (
    <>
      {metadata && <>Updated {formatRelative(metadata.updated_at)}</>}
      {metadata && !metadata.decryptable && (
        <span className="ml-2 text-destructive">decryption failing</span>
      )}
      {isZernio && <ZernioHelperBits />}
    </>
  )
}

function ZernioHelperBits() {
  const { data } = useZernioHealth()
  if (!data) return null
  const bits: string[] = []
  if (data.lastSyncAt) {
    bits.push(`Last synced ${formatRelative(data.lastSyncAt)}`)
    if (data.lastSyncStatus) bits.push(data.lastSyncStatus)
  } else {
    bits.push('Never synced')
  }
  bits.push(`${data.accountCount} account${data.accountCount === 1 ? '' : 's'}`)
  return <span> · {bits.join(' · ')}</span>
}

type EditKeyModalProps = {
  open: boolean
  onClose: () => void
  def: SecretDef
  configured: boolean
}

function EditKeyModal({ open, onClose, def, configured }: EditKeyModalProps) {
  const [value, setValue] = useState('')
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const upsert = useUpsertSecret()
  const del = useDeleteSecret()

  useEffect(() => {
    if (!open) {
      setValue('')
      setConfirmingDelete(false)
      upsert.reset()
      del.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const busy = upsert.isPending || del.isPending

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!value.trim()) return
    upsert.mutate(
      { name: def.name, value },
      {
        onSuccess: () => onClose(),
      },
    )
  }

  const handleDelete = () => {
    del.mutate(def.name, {
      onSuccess: () => onClose(),
    })
  }

  return (
    <ModalContainer
      isOpen={open}
      onClose={() => {
        if (!busy) onClose()
      }}
      title={`${configured ? 'Replace' : 'Set'} ${def.label} API key`}
      size="default"
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4" noValidate>
        <p className="text-sm text-primary-foreground">{def.description}</p>
        <div className="flex flex-col gap-2">
          <Label htmlFor={`secret-modal-${def.name}`}>API key</Label>
          <Textarea
            id={`secret-modal-${def.name}`}
            autoComplete="off"
            spellCheck={false}
            placeholder={def.inputPlaceholder}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            disabled={busy}
            autoFocus
            className="font-mono break-all min-h-20"
          />
          <p className="text-xs text-tertiary-foreground">
            Stored encrypted. The value is never read back.<br/>Replacing this key overwrites the
            stored secret.
          </p>
          {upsert.error && (
            <p className="text-xs text-destructive">{upsert.error.message}</p>
          )}
          {del.error && (
            <p className="text-xs text-destructive">{del.error.message}</p>
          )}
        </div>
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            {configured && !confirmingDelete && (
              <Button
                type="button"
                variant="destructive"
                size="default"
                onClick={() => setConfirmingDelete(true)}
                disabled={busy}
              >
                DELETE
              </Button>
            )}
            {configured && confirmingDelete && (
              <>
                <Button
                  type="button"
                  variant="destructiveInverted"
                  size="default"
                  onClick={handleDelete}
                  loading={del.isPending}
                  disabled={busy}
                >
                  CONFIRM DELETE
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="default"
                  onClick={() => setConfirmingDelete(false)}
                  disabled={busy}
                >
                  KEEP
                </Button>
              </>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="ghost"
              size="default"
              onClick={onClose}
              disabled={busy}
            >
              CANCEL
            </Button>
            <Button
              type="submit"
              variant="defaultInverted"
              size="default"
              loading={upsert.isPending}
              disabled={!value.trim() || busy}
            >
              {configured ? 'REPLACE' : 'SAVE'}
            </Button>
          </div>
        </div>
      </form>
    </ModalContainer>
  )
}

function ZernioStatePill() {
  const { data } = useZernioHealth()
  if (!data) return null
  return <StatePill state={data.state} />
}

function StatePill({ state }: { state: ZernioState }) {
  const tone =
    state === 'ok'
      ? 'bg-positive/15 text-positive'
      : state === 'degraded'
        ? 'bg-negative/15 text-negative'
        : 'bg-quaternary text-tertiary-foreground'
  const label = state === 'ok' ? 'Healthy' : state === 'degraded' ? 'Reconnecting' : 'Disabled'
  return <span className={cn('text-xs px-2 py-0.5', tone)}>{label}</span>
}

function formatRelative(iso: string): string {
  const then = new Date(iso).getTime()
  if (!Number.isFinite(then)) return iso
  const diff = Date.now() - then
  const sec = Math.round(diff / 1000)
  if (sec < 60) return 'just now'
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}h ago`
  const day = Math.round(hr / 24)
  if (day < 30) return `${day}d ago`
  return new Date(iso).toLocaleDateString()
}

export const ApiKeysSection = memo(ApiKeysSectionComponent)
