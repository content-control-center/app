import portfolioEmptyImage from '@/assets/portfolios/portfolio-folder-empty.webp'
import { Button } from '@/components/ui/button.tsx'
import { Icon } from '@/components/ui/icon.tsx'

export function CampaignEmptyState() {

  return (
    <div className={'relative'}>
      <div
        className={
          'absolute inset-0 bg-linear-to-b from-table-header to-background flex flex-col justify-center items-center'
        }
      >
        <div className="flex flex-col gap-5 justify-center items-center">
          <div className={'mb-5'}>
            <img src={portfolioEmptyImage} alt="Empty portfolio" className="w-40 h-auto" />
          </div>
          <div className="text-tertiary-foreground text-center space-y-2">
            <div className={'text-2xl/8 text-foreground font-display font-medium'}>
              Your Portfolio is Empty
            </div>
            <div>Add holdings to set up your portfolio structure</div>
          </div>
          <div>
            <Button variant={'defaultInverted'} onClick={()=>{}}>
              <Icon name={'plus'} className={'size-4 stroke-[2px]'} />
              <span>ADD SYMBOL</span>
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
