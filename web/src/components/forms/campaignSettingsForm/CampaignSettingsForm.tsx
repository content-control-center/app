import { useNavigate } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'

import { Input } from '@/components/ui/input'
import { DatePicker } from '@/components/ui/date-picker'
import { TagsInput } from '@/components/ui/tags-input'
import { Button } from '@/components/ui/button'
import { Trash } from '@phosphor-icons/react'
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
import { useDeleteCampaign } from '@/hooks/useCampaigns'
import type { Campaign } from '@/types/campaigns'
import { useCampaignAutosave, toNumberOrNull, toISODateTime } from '../campaignBriefForm/shared'
import { PlatformsControl } from './PlatformsControl'

const numericString = z
  .string()
  .refine((v) => v === '' || Number.isFinite(Number(v)), 'Must be a number')

const settingsSchema = z.object({
  name: z.string(),
  start_date: z.string().nullable(),
  end_date: z.string().nullable(),
  estimated_post_count: numericString,
  budget: numericString,
  currency: z.string(),
  language: z.string(),
  tag_ids: z.array(z.string()),
  target_platforms: z.array(
    z.object({
      id: z.string(),
      post_types: z.array(z.string()),
    }),
  ),
})

type SettingsFormValues = z.infer<typeof settingsSchema>

function defaultValues(campaign: Campaign): SettingsFormValues {
  return {
    name: campaign.name,
    start_date: campaign.start_date,
    end_date: campaign.end_date,
    estimated_post_count:
      campaign.estimated_post_count == null ? '' : String(campaign.estimated_post_count),
    budget: campaign.budget == null ? '' : String(campaign.budget),
    currency: campaign.currency,
    language: campaign.language,
    tag_ids: campaign.tag_ids ?? [],
    target_platforms: campaign.target_platforms ?? [],
  }
}

type Props = {
  campaign: Campaign
  onFlushRef?: (flush: () => void) => void
  onClose?: () => void
}

export function CampaignSettingsForm({ campaign, onFlushRef, onClose }: Props) {
  const form = useForm<SettingsFormValues>({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    resolver: zodResolver(settingsSchema as any),
    defaultValues: defaultValues(campaign),
  })

  const { mutate: deleteCampaign, isPending: deleting } = useDeleteCampaign()
  const navigate = useNavigate()

  const targetPlatforms = form.watch('target_platforms')
  const postTypeCount = targetPlatforms.reduce(
    (acc, p) => acc + p.post_types.length,
    0,
  )
  const initialPostTypeCount = (campaign.target_platforms ?? []).reduce(
    (acc, p) => acc + p.post_types.length,
    0,
  )

  const { error } = useCampaignAutosave({
    campaign,
    form,
    buildOverrides: (v) => ({
      name: v.name.trim() === '' ? ' ' : v.name,
      start_date: toISODateTime(v.start_date),
      end_date: toISODateTime(v.end_date),
      estimated_post_count: toNumberOrNull(v.estimated_post_count),
      budget: toNumberOrNull(v.budget),
      currency: v.currency,
      language: v.language,
      tag_ids: v.tag_ids,
      target_platforms: v.target_platforms,
    }),
    onFlushRef,
  })

  const handleDelete = () => {
    const displayName = campaign.name.trim() === '' ? 'this campaign' : `"${campaign.name}"`
    if (!window.confirm(`Delete ${displayName}? This cannot be undone.`)) return
    deleteCampaign(campaign.id, {
      onSuccess: () => {
        navigate({ to: '/campaigns' })
      },
    })
  }

  return (
    <Form {...form}>
      <form noValidate autoComplete="off" className="h-full">
        <RailPanel title="General settings" onClose={onClose}>
        <Collapse title="BASIC" defaultOpen>
          <div className="flex flex-col gap-4 pt-2 pb-4">
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
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="start_date"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Start date</FormLabel>
                    <DatePicker value={field.value} onChange={field.onChange} />
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
                    <DatePicker value={field.value} onChange={field.onChange} />
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name="tag_ids"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Tags</FormLabel>
                  <TagsInput value={field.value} onChange={field.onChange} />
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </Collapse>

        <Collapse title="ADVANCED">
          <div className="flex flex-col gap-4 pt-2 pb-4">
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="budget"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Budget</FormLabel>
                    <FormControl>
                      <Input type="number" min={0} step="0.01" placeholder="e.g. 5000" {...field} />
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
                      <Input placeholder="USD" {...field} />
                    </FormControl>
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
                      <Input type="number" min={0} placeholder="e.g. 12" {...field} />
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
                      <Input placeholder="en" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>
        </Collapse>

        <Collapse
          title="PLATFORMS & POST TYPES"
          meta={postTypeCount}
          defaultOpen={initialPostTypeCount === 0}
        >
          <div className="flex flex-col pt-2 pb-4">
            <FormField
              control={form.control}
              name="target_platforms"
              render={({ field }) => (
                <FormItem>
                  <PlatformsControl
                    value={field.value}
                    onChange={field.onChange}
                  />
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
              <Trash className="size-4" />
              <span>DELETE CAMPAIGN</span>
            </Button>
          </div>
        </Collapse>

        {error && <span className="text-xs text-destructive">{error.message}</span>}
        </RailPanel>
      </form>
    </Form>
  )
}

