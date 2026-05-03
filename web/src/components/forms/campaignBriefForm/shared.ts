import { useCallback, useEffect, useRef, useState } from 'react'
import type { UseFormReturn, FieldValues } from 'react-hook-form'
import { useUpdateCampaign } from '@/hooks/useCampaigns'
import type { Campaign, UpdateCampaignPayload } from '@/types/campaigns'

export function campaignToPayload(
  campaign: Campaign,
  overrides: Partial<UpdateCampaignPayload> = {},
): UpdateCampaignPayload {
  return {
    name: campaign.name,
    campaign_type_id: campaign.campaign_type_id,
    description: campaign.description,
    target_persona: campaign.target_persona,
    key_messages: campaign.key_messages,
    tone_guidelines: campaign.tone_guidelines,
    use_assets: campaign.use_assets,
    asset_ids: campaign.asset_ids,
    target_platforms: campaign.target_platforms,
    status: campaign.status,
    start_date: campaign.start_date,
    end_date: campaign.end_date,
    estimated_post_count: campaign.estimated_post_count,
    budget: campaign.budget,
    currency: campaign.currency,
    language: campaign.language,
    tag_ids: campaign.tag_ids,
    ...overrides,
  }
}

export function toNumberOrNull(s: string): number | null {
  if (s.trim() === '') return null
  const n = Number(s)
  return Number.isFinite(n) ? n : null
}

export function toISODateTime(value: string | null): string | null {
  if (!value) return null
  if (value.includes('T')) return value
  return `${value}T00:00:00Z`
}

type UseCampaignAutosaveArgs<T extends FieldValues> = {
  campaign: Campaign
  form: UseFormReturn<T>
  buildOverrides: (values: T) => Partial<UpdateCampaignPayload>
  onValuesChange?: (values: T, info: { name?: string }) => void
  onFlushRef?: (flush: () => void) => void
}

export function useCampaignAutosave<T extends FieldValues>({
  campaign,
  form,
  buildOverrides,
  onValuesChange,
  onFlushRef,
}: UseCampaignAutosaveArgs<T>) {
  const { mutate: updateCampaign, error, reset } = useUpdateCampaign()
  const editVersionRef = useRef(0)
  const [editVersion, setEditVersion] = useState(0)
  const [savedVersion, setSavedVersion] = useState(0)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const isDirty = editVersion !== savedVersion

  const saveNow = useCallback(() => {
    const v = editVersionRef.current
    void form.handleSubmit((values) => {
      const payload = campaignToPayload(campaign, buildOverrides(values))
      updateCampaign(
        { id: campaign.id, payload },
        { onSuccess: () => setSavedVersion(v) },
      )
    })()
  }, [campaign, form, updateCampaign, buildOverrides])

  const scheduleSave = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(saveNow, 500)
  }, [saveNow])

  const flushSave = useCallback(() => {
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current)
      saveTimerRef.current = null
      saveNow()
    }
  }, [saveNow])

  useEffect(() => {
    onFlushRef?.(flushSave)
  }, [onFlushRef, flushSave])

  useEffect(() => {
    const sub = form.watch((values, info) => {
      onValuesChange?.(values as T, info)
      editVersionRef.current += 1
      setEditVersion(editVersionRef.current)
      scheduleSave()
    })
    return () => sub.unsubscribe()
  }, [form, scheduleSave, onValuesChange])

  useEffect(() => {
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    }
  }, [])

  return { isDirty, error, resetError: reset, flushSave }
}
