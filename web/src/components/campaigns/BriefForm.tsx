import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'

import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { DatePicker } from '@/components/ui/date-picker'
import { TagsInput } from '@/components/ui/tags-input'
import { Checkbox } from '@/components/ui/checkbox'
import { Icon, type IconName } from '@/components/ui/icon'

const TYPE_ICON: Record<string, IconName> = {
  awareness: 'nav_screening',
  engagement: 'nav_ideas',
  conversion: 'nav_strategy',
  retention: 'nav_watchlist',
}

function typeIcon(name: string): IconName {
  return TYPE_ICON[name.toLowerCase()] ?? 'nav_dashboard'
}
import { cn } from '@/lib'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { useUpdateCampaign, useCampaignTypes } from '@/hooks/useCampaigns'
import type { Campaign, UpdateCampaignPayload } from '@/types/campaigns'

const numericString = z
  .string()
  .refine((v) => v === '' || Number.isFinite(Number(v)), 'Must be a number')

const briefSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  campaign_type_id: z.string().min(1, 'Campaign type is required'),
  description: z.string(),
  target_persona: z.string(),
  key_messages: z.string(),
  tone_guidelines: z.string(),
  start_date: z.string().nullable(),
  end_date: z.string().nullable(),
  estimated_post_count: numericString,
  budget: numericString,
  currency: z.string(),
  language: z.string(),
  use_pieces: z.boolean(),
  pieces_ids: z.string(),
  tag_ids: z.array(z.string()),
})

type BriefFormValues = z.infer<typeof briefSchema>

function defaultValues(campaign: Campaign): BriefFormValues {
  return {
    name: campaign.name,
    campaign_type_id: campaign.campaign_type_id,
    description: campaign.description,
    target_persona: campaign.target_persona,
    key_messages: campaign.key_messages,
    tone_guidelines: campaign.tone_guidelines,
    start_date: campaign.start_date,
    end_date: campaign.end_date,
    estimated_post_count:
      campaign.estimated_post_count == null ? '' : String(campaign.estimated_post_count),
    budget: campaign.budget == null ? '' : String(campaign.budget),
    currency: campaign.currency,
    language: campaign.language,
    use_pieces: campaign.use_pieces,
    pieces_ids: campaign.pieces_ids.join(', '),
    tag_ids: campaign.tag_ids,
  }
}

function parseCsv(s: string): string[] {
  return s
    .split(',')
    .map((x) => x.trim())
    .filter((x) => x.length > 0)
}

function toNumberOrNull(s: string): number | null {
  if (s.trim() === '') return null
  const n = Number(s)
  return Number.isFinite(n) ? n : null
}

type BriefFormProps = {
  campaign: Campaign
  onTitleChange?: (title: string) => void
  onDirtyChange?: (dirty: boolean) => void
}

function toPayload(values: BriefFormValues, campaign: Campaign): UpdateCampaignPayload {
  return {
    name: values.name.trim(),
    campaign_type_id: values.campaign_type_id,
    description: values.description,
    target_persona: values.target_persona,
    key_messages: values.key_messages,
    tone_guidelines: values.tone_guidelines,
    status: campaign.status,
    start_date: values.start_date,
    end_date: values.end_date,
    estimated_post_count: toNumberOrNull(values.estimated_post_count),
    budget: toNumberOrNull(values.budget),
    currency: values.currency,
    language: values.language,
    use_pieces: values.use_pieces,
    pieces_ids: parseCsv(values.pieces_ids),
    tag_ids: values.tag_ids,
    target_platforms: campaign.target_platforms,
  }
}

export function BriefForm({ campaign, onTitleChange, onDirtyChange }: BriefFormProps) {
  const form = useForm<BriefFormValues>({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    resolver: zodResolver(briefSchema as any),
    defaultValues: defaultValues(campaign),
  })

  const { data: types, isLoading: typesLoading } = useCampaignTypes()
  const { mutate: updateCampaign, error, reset } = useUpdateCampaign()

  const usePieces = form.watch('use_pieces')

  const editVersionRef = useRef(0)
  const [editVersion, setEditVersion] = useState(0)
  const [savedVersion, setSavedVersion] = useState(0)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const isDirty = editVersion !== savedVersion

  useEffect(() => {
    onDirtyChange?.(isDirty)
  }, [isDirty, onDirtyChange])

  useEffect(() => {
    onTitleChange?.(campaign.name)
    // only on mount and when a new campaign is loaded
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [campaign.id])

  const scheduleSave = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    const v = editVersionRef.current
    saveTimerRef.current = setTimeout(() => {
      void form.handleSubmit((values) => {
        const payload = toPayload(values, campaign)
        updateCampaign(
          { id: campaign.id, payload },
          {
            onSuccess: () => setSavedVersion(v),
          }
        )
      })()
    }, 500)
  }, [campaign, form, updateCampaign])

  useEffect(() => {
    const sub = form.watch((values, info) => {
      if (info.name === 'name') onTitleChange?.(values.name ?? '')
      editVersionRef.current += 1
      setEditVersion(editVersionRef.current)
      scheduleSave()
    })
    return () => sub.unsubscribe()
  }, [form, scheduleSave, onTitleChange])

  useEffect(() => {
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    }
  }, [])

  const disabled = false

  return (
    <Form {...form}>
      <form
        className="flex flex-col gap-6"
        noValidate
        autoComplete="off"
      >
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-start">
          <Card>
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Campaign name</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      name="campaign-name"
                      autoComplete="off"
                      placeholder="e.g. Spring product launch"
                      disabled={disabled}
                      onChange={(e) => {
                        field.onChange(e)
                        if (error) reset()
                      }}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="campaign_type_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Campaign type</FormLabel>
                  <div className="grid grid-flow-col auto-cols-fr gap-2">
                    {(types ?? []).map((t) => {
                      const selected = field.value === t.id
                      return (
                        <button
                          key={t.id}
                          type="button"
                          onClick={() => field.onChange(t.id)}
                          disabled={typesLoading}
                          aria-pressed={selected}
                          className={cn(
                            'flex flex-col items-center justify-center gap-2 rounded-md border px-3 py-4 cursor-pointer transition-colors',
                            'disabled:pointer-events-none disabled:opacity-50',
                            selected
                              ? 'border-foreground text-foreground'
                              : 'border-quaternary text-secondary-foreground hover:border-foreground hover:text-foreground'
                          )}
                        >
                          <span
                            className={cn(
                              'flex items-center justify-center rounded-md size-8 transition-colors',
                              selected
                                ? 'bg-foreground text-background'
                                : 'bg-transparent'
                            )}
                          >
                            <Icon name={typeIcon(t.name)} className="size-5" />
                          </span>
                          <span className="text-[11px] font-medium tracking-[0.08em] uppercase">
                            {t.label || t.name}
                          </span>
                        </button>
                      )
                    })}
                  </div>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Description</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="What is this campaign about and why does it matter?"
                      disabled={disabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="target_persona"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Target persona</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Who are we talking to? Role, goals, pain points."
                      disabled={disabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="key_messages"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Key messages</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="The core points every piece of content should land."
                      disabled={disabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="tone_guidelines"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Tone guidelines</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Voice, style, words to use and avoid."
                      disabled={disabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </Card>

          <div className="flex flex-col gap-6">
            <Card>
              <div className="grid grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="start_date"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Start date</FormLabel>
                      <DatePicker
                        value={field.value}
                        onChange={field.onChange}
                        disabled={disabled}
                      />
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="end_date"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>End date</FormLabel>
                      <DatePicker
                        value={field.value}
                        onChange={field.onChange}
                        disabled={disabled}
                      />
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="estimated_post_count"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Estimated post count</FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min={0}
                          placeholder="e.g. 12"
                          {...field}
                          disabled={disabled}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="language"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Language</FormLabel>
                      <FormControl>
                        <Input placeholder="en" {...field} disabled={disabled} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="budget"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Budget</FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min={0}
                          step="0.01"
                          placeholder="e.g. 5000"
                          {...field}
                          disabled={disabled}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="currency"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Currency</FormLabel>
                      <FormControl>
                        <Input placeholder="USD" {...field} disabled={disabled} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </Card>

            <Card>
              <FormField
                control={form.control}
                name="use_pieces"
                render={({ field }) => (
                  <FormItem>
                    <label className="flex items-center gap-2 cursor-pointer select-none">
                      <Checkbox
                        checked={field.value}
                        onCheckedChange={(c) => field.onChange(c === true)}
                        disabled={disabled}
                      />
                      <span className="text-[13px] text-input-label">
                        Use content bank pieces
                      </span>
                    </label>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {usePieces && (
                <FormField
                  control={form.control}
                  name="pieces_ids"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Piece IDs</FormLabel>
                      <FormControl>
                        <Input
                          placeholder="piece-1, piece-2"
                          {...field}
                          disabled={disabled}
                        />
                      </FormControl>
                      <FormDescription>Comma-separated</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </Card>

            <Card>
              <FormField
                control={form.control}
                name="tag_ids"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Tags</FormLabel>
                    <TagsInput
                      value={field.value}
                      onChange={field.onChange}
                      disabled={disabled}
                    />
                    <FormMessage />
                  </FormItem>
                )}
              />
            </Card>
          </div>
        </div>

        {error && <span className="text-xs text-destructive">{error.message}</span>}
      </form>
    </Form>
  )
}

function Card({ children }: { children: ReactNode }) {
  return (
    <div className="bg-white rounded-md p-6 flex flex-col gap-4">{children}</div>
  )
}
