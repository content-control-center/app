import { useCallback, useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { CampaignBriefForm } from '@/components/forms/campaignBriefForm'
import { useCampaign } from '@/hooks/useCampaigns'
import { PageLoader } from '@/components/page-primitives/PageLoader'
import { PageHeader } from '@/components/page-primitives/PageHeader.tsx'
import { useOverlayStore } from '@/stores/overlayStore'

type CampaignSettingsOverlayProps = {
  campaignId: string
}

export function CampaignSettingsOverlay({
  campaignId,
}: CampaignSettingsOverlayProps) {
  const { data: campaign, isLoading } = useCampaign(campaignId)
  const flushRef = useRef<(() => void) | null>(null)

  const handleFlushRef = useCallback((flush: () => void) => {
    flushRef.current = flush
  }, [])

  useEffect(() => {
    const flush = () => flushRef.current?.()
    useOverlayStore.getState().registerBeforeClose('campaign-settings', flush)
    return () => {
      useOverlayStore.getState().unregisterBeforeClose('campaign-settings')
    }
  }, [])

  if (isLoading || !campaign) {
    return <PageLoader />
  }

  const displayName = campaign.name.trim() === '' ? 'Untitled' : campaign.name

  return (
    <ScrollArea className="h-full" type="scroll" scrollHideDelay={350}>
      <PageHeader
        title={`${displayName} — Settings`}
        className={"pt-6"}
      />
      <div className="px-6 py-6">
        <CampaignBriefForm campaign={campaign} onFlushRef={handleFlushRef} />
      </div>
    </ScrollArea>
  )
}
