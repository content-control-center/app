import { useEffect, useMemo } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'

import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { DatePicker } from '@/components/ui/date-picker'
import { Button } from '@/components/ui/button'
import { TrashIcon } from '@phosphor-icons/react'
import { TextSelect } from '@/components/ui/text-select'
import { Collapse } from '@/components/ui/collapse'
import { RailPanel } from '@/components/page-primitives/RailPanel'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { useCampaign } from '@/hooks/useCampaigns'
import { useDeletePost } from '@/hooks/usePosts'
import { cn } from '@/lib'
import type { Post } from '@/types/posts'
import { CampaignPostTypeSelect } from './CampaignPostTypeSelect'

const NO_PHASE = '__none__'
const DEFAULT_HOUR = 9
const DEFAULT_MINUTE = 0

const schema = z.object({
  platform_id: z.string(),
  platform_post_type: z.string(),
  scheduled_at: z.string().nullable(),
  target_audience_notes: z.string(),
  campaign_type_phase_id: z.string(),
})

type FormValues = z.infer<typeof schema>

function docToFormValues(doc: Post): FormValues {
  return {
    platform_id: doc.platform_id,
    platform_post_type: doc.platform_post_type,
    scheduled_at: doc.scheduled_at,
    target_audience_notes: doc.target_audience_notes,
    campaign_type_phase_id: doc.campaign_type_phase_id ?? NO_PHASE,
  }
}

function toLocalParts(iso: string | null): { dateStr: string; timeStr: string } {
  if (!iso) return { dateStr: '', timeStr: '' }
  const d = new Date(iso)
  if (isNaN(d.getTime())) return { dateStr: '', timeStr: '' }
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  return { dateStr: `${y}-${m}-${day}`, timeStr: `${hh}:${mm}` }
}

function fromLocalParts(dateStr: string, timeStr: string): string | null {
  if (!dateStr) return null
  const [y, m, d] = dateStr.split('-').map(Number)
  const [hh, mm] = timeStr
    ? timeStr.split(':').map(Number)
    : [DEFAULT_HOUR, DEFAULT_MINUTE]
  const local = new Date(y, m - 1, d, hh ?? 0, mm ?? 0, 0, 0)
  return local.toISOString()
}

function getLocalTimezoneLabel(): string {
  try {
    const parts = new Intl.DateTimeFormat(undefined, {
      timeZoneName: 'short',
    }).formatToParts(new Date())
    return parts.find((p) => p.type === 'timeZoneName')?.value ?? 'local time'
  } catch {
    return 'local time'
  }
}

type Props = {
  doc: Post
  changeDoc: (fn: (p: Post) => void) => void
  onClose?: () => void
}

export function PostSettingsForm({ doc, changeDoc, onClose }: Props) {
  const form = useForm<FormValues>({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    resolver: zodResolver(schema as any),
    defaultValues: docToFormValues(doc),
  })

  const { data: campaign } = useCampaign(doc.campaign_id)
  const { mutate: deletePost, isPending: deleting } = useDeletePost(doc.campaign_id)
  const navigate = useNavigate()

  const platformId = form.watch('platform_id')
  const platformPostType = form.watch('platform_post_type')
  const tzLabel = useMemo(() => getLocalTimezoneLabel(), [])

  useEffect(() => {
    const sub = form.watch((values, info) => {
      if (!info.name) return
      changeDoc((d) => {
        switch (info.name) {
          case 'platform_id':
            if (values.platform_id) d.platform_id = values.platform_id
            if (values.platform_post_type) d.platform_post_type = values.platform_post_type
            break
          case 'platform_post_type':
            if (values.platform_post_type) d.platform_post_type = values.platform_post_type
            break
          case 'scheduled_at':
            d.scheduled_at = values.scheduled_at ?? null
            break
          case 'target_audience_notes':
            d.target_audience_notes = values.target_audience_notes ?? ''
            break
          case 'campaign_type_phase_id':
            d.campaign_type_phase_id =
              values.campaign_type_phase_id === NO_PHASE || !values.campaign_type_phase_id
                ? null
                : values.campaign_type_phase_id
            break
        }
      })
    })
    return () => sub.unsubscribe()
  }, [form, changeDoc])

  const phaseOptions = useMemo(() => {
    const phases = campaign?.campaign_type?.phases ?? []
    return [
      { id: NO_PHASE, displayValue: 'No phase' },
      ...phases.map((ph) => ({ id: ph.id, displayValue: ph.name })),
    ]
  }, [campaign])

  const handleDelete = () => {
    const label = doc.title.trim() === '' ? 'this post' : `"${doc.title}"`
    if (!window.confirm(`Delete ${label}? This cannot be undone.`)) return
    deletePost(doc.id, {
      onSuccess: () => {
        navigate({
          to: '/campaigns/$campaignId',
          params: { campaignId: doc.campaign_id },
        })
      },
    })
  }

  return (
    <Form {...form}>
      <form noValidate autoComplete="off" className="h-full">
        <RailPanel title="Post settings" onClose={onClose}>
          <Collapse title="BASIC" defaultOpen>
            <div className="flex flex-col gap-4 pt-2 pb-4">
              <FormItem>
                <FormLabel>Post type</FormLabel>
                <FormControl>
                  <CampaignPostTypeSelect
                    campaign={campaign}
                    platformId={platformId}
                    postType={platformPostType}
                    onChange={(pid, slug) => {
                      form.setValue('platform_id', pid, { shouldDirty: true })
                      form.setValue('platform_post_type', slug, {
                        shouldDirty: true,
                      })
                    }}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
              <FormField
                control={form.control}
                name="scheduled_at"
                render={({ field }) => {
                  const { dateStr, timeStr } = toLocalParts(field.value)
                  return (
                    <FormItem>
                      <FormLabel>
                        Publish date and time
                        <span className="ml-2 text-xs font-normal text-tertiary-foreground">
                          ({tzLabel} time zone)
                        </span>
                        {field.value && (
                          <button
                            type="button"
                            onClick={() => field.onChange(null)}
                            className="ml-2 text-xs font-normal text-tertiary-foreground hover:text-foreground transition-colors cursor-pointer underline"
                          >
                            Clear
                          </button>
                        )}
                      </FormLabel>
                      <div className="flex items-center gap-2">
                        <div className="flex-1 min-w-0">
                          <DatePicker
                            value={dateStr ? `${dateStr}T00:00:00` : null}
                            onChange={(nextDate) =>
                              field.onChange(fromLocalParts(nextDate ?? '', timeStr))
                            }
                          />
                        </div>
                        <div className="relative w-24">
                          <Input
                            type="time"
                            value={timeStr}
                            onChange={(e) =>
                              field.onChange(fromLocalParts(dateStr, e.target.value))
                            }
                            disabled={!dateStr}
                            data-empty={!timeStr}
                            className={cn(
                              'w-24 appearance-none',
                              '[&::-webkit-calendar-picker-indicator]:hidden [&::-webkit-calendar-picker-indicator]:appearance-none',
                              "data-[empty=true]:[&::-webkit-datetime-edit]:text-transparent",
                            )}
                          />
                          {!timeStr && (
                            <span className="pointer-events-none absolute inset-y-0 left-4 flex items-center text-[14px] font-medium text-tertiary-foreground">
                              12:00
                            </span>
                          )}
                        </div>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )
                }}
              />
            </div>
          </Collapse>

          <Collapse title="ADVANCED">
            <div className="flex flex-col gap-4 pt-2 pb-4">
              <FormField
                control={form.control}
                name="campaign_type_phase_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Campaign phase</FormLabel>
                    <FormControl>
                      <TextSelect
                        variant="default"
                        value={field.value}
                        onValueChange={field.onChange}
                        elements={phaseOptions}
                        placeholder="No phase"
                        disabled={phaseOptions.length <= 1}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="target_audience_notes"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Target audience notes</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder="Who should this reach?"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </Collapse>

          <Collapse title="DANGER ZONE">
            <div className="pt-2 pb-4">
              <Button
                type="button"
                variant="destructiveInverted"
                onClick={handleDelete}
                loading={deleting}
              >
                <TrashIcon className="size-4" />
                <span>DELETE POST</span>
              </Button>
            </div>
          </Collapse>
        </RailPanel>
      </form>
    </Form>
  )
}
