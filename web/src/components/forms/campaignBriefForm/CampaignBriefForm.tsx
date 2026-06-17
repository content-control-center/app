import { type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'

import { Textarea } from '@/components/ui/textarea'
import { BookmarkSimpleIcon, ChatCircleIcon, EyeIcon, GaugeIcon, TargetIcon, type Icon as PhosphorIcon } from '@phosphor-icons/react'
import { cn } from '@/lib'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { useCampaignTypes } from '@/hooks/useCampaigns'
import type { Campaign } from '@/types/campaigns'
import { useCampaignAutosave } from './shared'

const TYPE_ICON: Record<string, PhosphorIcon> = {
  awareness: EyeIcon,
  engagement: ChatCircleIcon,
  conversion: TargetIcon,
  retention: BookmarkSimpleIcon,
}

function typeIcon(name: string): PhosphorIcon {
  return TYPE_ICON[name.toLowerCase()] ?? GaugeIcon
}

const briefSchema = z.object({
  campaign_type_id: z.string().min(1, 'Campaign type is required'),
  description: z.string(),
  target_persona: z.string(),
  key_messages: z.string(),
  tone_guidelines: z.string(),
})

type BriefFormValues = z.infer<typeof briefSchema>

function defaultValues(campaign: Campaign): BriefFormValues {
  return {
    campaign_type_id: campaign.campaign_type_id,
    description: campaign.description,
    target_persona: campaign.target_persona,
    key_messages: campaign.key_messages,
    tone_guidelines: campaign.tone_guidelines,
  }
}

type BriefFormProps = {
  campaign: Campaign
  onFlushRef?: (flush: () => void) => void
}

export function BriefForm({ campaign, onFlushRef }: BriefFormProps) {
  const form = useForm<BriefFormValues>({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    resolver: zodResolver(briefSchema as any),
    defaultValues: defaultValues(campaign),
  })

  const { data: types, isLoading: typesLoading } = useCampaignTypes()

  const { error, resetError } = useCampaignAutosave({
    campaign,
    form,
    buildOverrides: (v) => ({
      campaign_type_id: v.campaign_type_id,
      description: v.description,
      target_persona: v.target_persona,
      key_messages: v.key_messages,
      tone_guidelines: v.tone_guidelines,
    }),
    onFlushRef,
  })

  return (
    <Form {...form}>
      <form className="flex flex-col gap-6" noValidate autoComplete="off">
        <Card>
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
                        onClick={() => {
                          field.onChange(t.id)
                          if (error) resetError()
                        }}
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
                          {(() => {
                            const TypeIcon = typeIcon(t.name)
                            return <TypeIcon className="size-5" />
                          })()}
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
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </Card>

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
