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
  rightTabs?: CampaignTab[]
  onTabSelect: (tabId: string) => void
  action?: ReactNode
}

export function CampaignTabBar({
  activeTab,
  tabs,
  rightTabs,
  onTabSelect,
  action,
}: CampaignTabBarProps) {
  const isMobile = useIsMobile()
  const activeTriggerRef = useRef<HTMLButtonElement>(null)
  const allTabs = [...tabs, ...(rightTabs ?? [])]

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

  const renderTrigger = (tab: CampaignTab) => (
    <TabsTrigger
      variant="underline"
      key={tab.id}
      value={tab.id}
      ref={isMobile && tab.id === activeTab ? activeTriggerRef : undefined}
      onClick={(e) => {
        if (tab.id === activeTab) e.preventDefault()
        handleTabClick(tab.id)
      }}
    >
      {tab.label}
    </TabsTrigger>
  )

  if (isMobile) {
    return (
      <div className="flex items-center mx-3 mt-1 mb-0.5 gap-1 shrink-0">
        <div
          className="flex-1 overflow-x-auto border-b border-border"
          style={{ scrollbarWidth: 'none' }}
        >
          <Tabs value={activeTab} className="-mb-px">
            <TabsList variant="underline" size="lg">
              {allTabs.map(renderTrigger)}
            </TabsList>
          </Tabs>
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
    )
  }

  return (
    <div className="flex items-center justify-between mx-3 lg:mx-6 mt-2 mb-0.5 gap-4 shrink-0">
      <div className="flex-1 border-b border-border flex items-end justify-between">
        <Tabs value={activeTab} className="-mb-px">
          <TabsList variant="underline" size="lg">
            {tabs.map(renderTrigger)}
          </TabsList>
        </Tabs>
        {rightTabs && rightTabs.length > 0 && (
          <Tabs value={activeTab} className="-mb-px">
            <TabsList variant="underline" size="lg">
              {rightTabs.map(renderTrigger)}
            </TabsList>
          </Tabs>
        )}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  )
}
