import { useMemo } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'

import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { DatePicker } from '@/components/ui/date-picker'
import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'
import { TextSelect } from '@/components/ui/text-select'
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
import { usePlatforms } from '@/hooks/usePlatforms'
import { useDeletePost } from '@/hooks/usePosts'
import type { Post, PostCTAType, PostStatus } from '@/types/posts'
import { POST_STATUS_LABELS } from '@/types/posts'
import { usePostAutosave } from './shared'

const STATUS_OPTIONS: { id: PostStatus; displayValue: string }[] = (
  Object.keys(POST_STATUS_LABELS) as PostStatus[]
).map((id) => ({ id, displayValue: POST_STATUS_LABELS[id] }))

const CTA_OPTIONS: { id: PostCTAType; displayValue: string }[] = [
  { id: 'none', displayValue: 'None' },
  { id: 'link', displayValue: 'Link' },
  { id: 'button', displayValue: 'Button' },
]

const NO_PHASE = '__none__'

const schema = z.object({
  platform_id: z.string().min(1, 'Platform is required'),
  platform_post_type: z.string().min(1, 'Post type is required'),
  status: z.string().min(1),
  scheduled_at: z.string().nullable(),
  cta_type: z.string(),
  cta_url: z.string(),
  target_audience_notes: z.string(),
  campaign_type_phase_id: z.string(),
})

type FormValues = z.infer<typeof schema>

function defaultValues(post: Post): FormValues {
  return {
    platform_id: post.platform_id,
    platform_post_type: post.platform_post_type,
    status: post.status,
    scheduled_at: post.scheduled_at,
    cta_type: post.cta_type,
    cta_url: post.cta_url,
    target_audience_notes: post.target_audience_notes,
    campaign_type_phase_id: post.campaign_type_phase_id ?? NO_PHASE,
  }
}

type Props = {
  post: Post
  onClose?: () => void
}

export function PostSettingsForm({ post, onClose }: Props) {
  const form = useForm<FormValues>({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    resolver: zodResolver(schema as any),
    defaultValues: defaultValues(post),
  })

  const { data: platforms } = usePlatforms()
  const { data: campaign } = useCampaign(post.campaign_id)
  const { mutate: deletePost, isPending: deleting } = useDeletePost(post.campaign_id)
  const navigate = useNavigate()

  const platformId = form.watch('platform_id')

  usePostAutosave({
    post,
    form,
    buildOverrides: (v) => ({
      platform_id: v.platform_id,
      platform_post_type: v.platform_post_type,
      status: v.status as PostStatus,
      scheduled_at: v.scheduled_at,
      cta_type: v.cta_type as PostCTAType,
      cta_url: v.cta_url,
      target_audience_notes: v.target_audience_notes,
      campaign_type_phase_id:
        v.campaign_type_phase_id === NO_PHASE ? null : v.campaign_type_phase_id,
    }),
  })

  const platformOptions = useMemo(
    () =>
      (platforms ?? []).map((p) => ({ id: p.id, displayValue: p.name })),
    [platforms],
  )

  const postTypeOptions = useMemo(() => {
    const p = (platforms ?? []).find((x) => x.id === platformId)
    return Object.entries(p?.post_types ?? {}).map(([id, label]) => ({
      id,
      displayValue: label,
    }))
  }, [platforms, platformId])

  const phaseOptions = useMemo(() => {
    const phases = campaign?.campaign_type?.phases ?? []
    return [
      { id: NO_PHASE, displayValue: 'No phase' },
      ...phases.map((ph) => ({ id: ph.id, displayValue: ph.name })),
    ]
  }, [campaign])

  const handleDelete = () => {
    const label = post.title.trim() === '' ? 'this post' : `"${post.title}"`
    if (!window.confirm(`Delete ${label}? This cannot be undone.`)) return
    deletePost(post.id, {
      onSuccess: () => {
        navigate({
          to: '/campaigns/$campaignId',
          params: { campaignId: post.campaign_id },
        })
      },
    })
  }

  return (
    <Form {...form}>
      <form noValidate autoComplete="off" className="h-full">
        <RailPanel title="Post settings" onClose={onClose}>
          <div className="flex flex-col gap-4">
            <FormField
              control={form.control}
              name="status"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Status</FormLabel>
                  <FormControl>
                    <TextSelect
                      value={field.value}
                      onValueChange={field.onChange}
                      elements={STATUS_OPTIONS}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="platform_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Platform</FormLabel>
                    <FormControl>
                      <TextSelect
                        value={field.value}
                        onValueChange={(v) => {
                          field.onChange(v)
                          const next = platforms?.find((p) => p.id === v)
                          const firstType = Object.keys(next?.post_types ?? {})[0] ?? ''
                          form.setValue('platform_post_type', firstType, {
                            shouldDirty: true,
                          })
                        }}
                        elements={platformOptions}
                        placeholder="Select platform"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="platform_post_type"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Post type</FormLabel>
                    <FormControl>
                      <TextSelect
                        value={field.value}
                        onValueChange={field.onChange}
                        elements={postTypeOptions}
                        placeholder="Select type"
                        disabled={postTypeOptions.length === 0}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name="scheduled_at"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Scheduled date</FormLabel>
                  <DatePicker value={field.value} onChange={field.onChange} />
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="campaign_type_phase_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Campaign phase</FormLabel>
                  <FormControl>
                    <TextSelect
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
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="cta_type"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>CTA type</FormLabel>
                    <FormControl>
                      <TextSelect
                        value={field.value}
                        onValueChange={field.onChange}
                        elements={CTA_OPTIONS}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="cta_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>CTA URL</FormLabel>
                    <FormControl>
                      <Input placeholder="https://..." {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
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

          <div className="flex flex-col gap-3 pt-4 border-t border-border">
            <div className="flex flex-col gap-1">
              <span className="text-[13px] font-medium text-input-label">Danger zone</span>
              <span className="text-xs text-tertiary-foreground">
                Deleting a post removes it permanently. This cannot be undone.
              </span>
            </div>
            <Button
              type="button"
              variant="destructiveInverted"
              onClick={handleDelete}
              loading={deleting}
            >
              <Icon name="trash_bin" className="size-4" />
              <span>DELETE POST</span>
            </Button>
          </div>
        </RailPanel>
      </form>
    </Form>
  )
}
