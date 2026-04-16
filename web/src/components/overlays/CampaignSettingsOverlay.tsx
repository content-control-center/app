import { useCallback, useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { CampaignBriefForm } from '@/components/forms/campaignBriefForm'
import { useCampaign } from '@/hooks/useCampaigns'
import { PageLoader } from '@/components/page-primitives/PageLoader'
import { PageHeader } from '@/components/page-primitives/PageHeader.tsx'

type CampaignSettingsOverlayProps = {
  campaignId: string
  onClose: () => void
}

export function CampaignSettingsOverlay({
  campaignId,
  onClose,
}: CampaignSettingsOverlayProps) {
  const { data: campaign, isLoading } = useCampaign(campaignId)
  const flushRef = useRef<(() => void) | null>(null)

  const handleFlushRef = useCallback((flush: () => void) => {
    flushRef.current = flush
  }, [])

  const handleClose = useCallback(() => {
    flushRef.current?.()
    onClose()
  }, [onClose])

  useEffect(() => {
    return () => {
      flushRef.current?.()
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
        actions={
          <button
            onClick={handleClose}
            className="text-sm text-tertiary-foreground hover:text-foreground transition-colors cursor-pointer"
            aria-label="Close settings"
          >
            CLOSE
          </button>
        }
      />
      <div className="px-6 py-6">
        <CampaignBriefForm campaign={campaign} onFlushRef={handleFlushRef} />
      </div>
    </ScrollArea>
  )
}
