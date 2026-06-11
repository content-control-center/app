import { useNavigate } from '@tanstack/react-router'
import { useCampaigns } from '@/hooks/useCampaigns'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { XIcon } from '@phosphor-icons/react'

type CampaignSelectorProps = {
  onClose: () => void
}

export function CampaignSelector({ onClose }: CampaignSelectorProps) {
  const { data: campaigns, isLoading } = useCampaigns()
  const navigate = useNavigate()

  const handleSelect = (id: string) => {
    navigate({ to: '/campaigns/$campaignId', params: { campaignId: id } })
    onClose()
  }

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between px-4 pt-4 pb-2 shrink-0">
        <h2 className="text-lg font-display font-medium tracking-tight">Campaigns</h2>
        <Button variant="ghost" size="smIcon" onClick={onClose} aria-label="Close">
          <XIcon className="size-4" />
        </Button>
      </div>
      <ScrollArea className="flex-1 min-h-0" type="scroll">
        <ul className="flex flex-col gap-1 px-2 pb-4">
          {isLoading && (
            <li className="px-3 py-2 text-sm text-tertiary-foreground">Loading…</li>
          )}
          {!isLoading && (!campaigns || campaigns.length === 0) && (
            <li className="px-3 py-2 text-sm text-tertiary-foreground">No campaigns yet</li>
          )}
          {campaigns?.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                onClick={() => handleSelect(c.id)}
                className="w-full text-left px-3 py-2 rounded-md hover:bg-sidebar-secondary cursor-pointer"
              >
                <div className="text-sm font-medium truncate">
                  {c.name.trim() || 'Untitled campaign'}
                </div>
                <div className="text-xs text-tertiary-foreground">{c.status}</div>
              </button>
            </li>
          ))}
        </ul>
      </ScrollArea>
    </div>
  )
}
