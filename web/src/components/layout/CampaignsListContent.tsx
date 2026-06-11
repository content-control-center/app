//import { useSettingsStore } from '@/stores/settingsStore'
import { useEffect } from 'react'
import { Button } from '@/components/ui/button'
// import { useNavigate, useLocation } from '@tanstack/react-router'
import { useIsMobile } from '@/hooks/use-mobile.ts'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Plus } from '@phosphor-icons/react'



export function CampaignsListContent() {
  //const closeSecondaryNavbar = useSettingsStore((state) => state.closeSecondaryNavbar)
  // const navigate = useNavigate()
  // const location = useLocation()
  const isMobile = useIsMobile()
//  const createOverlay = useOverlay('create-campaign')

  useEffect(() => {
    // Separate "All Campaigns" from regular campaigns


    // Sort regular campaigns by lastUpdated (most recent first)


    // Always put "All Campaigns" first if it exists

  }, [])

  // const handleCampaignClick = (campaignId: string) => {
  //   // Extract the current tab path from the location (e.g., /holdings, /exposures)
  //
  //   closeSecondaryNavbar()
  // }


  return (
    <div className="flex flex-col h-full py-3 px-3 lg:pt-6 lg:pb-3 lg:px-8 overflow-x-hidden">
      <div className="h-8 lg:h-12">
        <h2 className="text-xl font-medium leading-8 truncate font-display tracking-tight lg:text-[2rem] lg:leading-[46px]">
          Campaigns
        </h2>
      </div>
      <ScrollArea className="flex-1 mt-4">
        <div className="flex flex-col gap-1">
          {/*{availablePortfoliosList.length === 0 ? (*/}
          {/*  <p className="text-sm text-tertiary-foreground">*/}
          {/*    There are no portfolios yet.*/}
          {/*    <br />*/}
          {/*    Go ahead and create a new one!*/}
          {/*  </p>*/}
          {/*) : (*/}
          {/*  availablePortfoliosList.map((portfolio) => {*/}
          {/*    if (!portfolio.calculations) return null*/}
          {/*    return (*/}
          {/*      <PortfolioCard*/}
          {/*        key={portfolio.id}*/}
          {/*        {...portfolio}*/}
          {/*        isExpanded={availablePortfoliosList.length <= 4}*/}
          {/*        onClick={() => handlePortfolioClick(portfolio.id)}*/}
          {/*      />*/}
          {/*    )*/}
          {/*  })*/}
          {/*)}*/}
        </div>
        {!isMobile && (
          <div className="sticky bottom-0 pt-2 bg-sidebar">
            <Button
              size={'lg'}
              variant={'link'}
              className="justify-start w-full px-0"
          //    onClick={() => createOverlay.open()}
            >
              <Plus />
              <span>CREATE CAMPAIGN</span>
            </Button>
          </div>
        )}
      </ScrollArea>
      {isMobile && (
        <div className="shrink-0 pt-3 bg-popover">
          <Button
            size={'lg'}
            variant={'outline'}
            className="w-full justify-between flex-row-reverse"
          //  onClick={() => createOverlay.open()}
          >
            <Plus />
            <span>CREATE PORTFOLIO</span>
          </Button>
        </div>
      )}
    </div>
  )
}
