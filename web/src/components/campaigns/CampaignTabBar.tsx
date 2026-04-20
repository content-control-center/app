import { useEffect, useRef, type ReactNode } from 'react'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useIsMobile } from '@/hooks/use-mobile'

export type CampaignTab = {
  id: string
  label: string
}

type CampaignTabBarProps = {
  activeTab: string
  tabs: CampaignTab[]
  onTabSelect: (tabId: string) => void
  action?: ReactNode
}

export function CampaignTabBar({
  activeTab,
  tabs,
  onTabSelect,
  action,
}: CampaignTabBarProps) {
  const isMobile = useIsMobile()
  const activeTriggerRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (isMobile) {
      activeTriggerRef.current?.scrollIntoView?.({
        behavior: 'smooth',
        block: 'nearest',
        inline: 'center',
      })
    }
  }, [activeTab, isMobile])

  const handleTabClick = (tabId: string) => {
    if (tabId === activeTab) return
    onTabSelect(tabId)
  }

  if (isMobile) {
    return (
      <div className="flex items-center mx-3 mt-1 mb-0.5 gap-1 shrink-0">
        <div
          className="flex-1 overflow-x-auto border-b border-border"
          style={{ scrollbarWidth: 'none' }}
        >
          <Tabs value={activeTab} className="-mb-px">
            <TabsList variant="underline" size="lg">
              {tabs.map((tab) => (
                <TabsTrigger
                  variant="underline"
                  key={tab.id}
                  value={tab.id}
                  ref={tab.id === activeTab ? activeTriggerRef : undefined}
                  onClick={() => handleTabClick(tab.id)}
                >
                  {tab.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
    )
  }

  return (
    <div className="flex items-center justify-between mx-3 lg:mx-6 mt-2 mb-0.5 gap-4 shrink-0">
      <div className="flex-1 border-b border-border">
        <Tabs value={activeTab} className="-mb-px">
          <TabsList variant="underline" size="lg">
            {tabs.map((tab) => (
              <TabsTrigger
                variant="underline"
                key={tab.id}
                value={tab.id}
                onClick={(e) => {
                  if (tab.id === activeTab) e.preventDefault()
                  handleTabClick(tab.id)
                }}
              >
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  )
}
