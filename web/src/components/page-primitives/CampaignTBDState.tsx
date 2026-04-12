import portfolioEmptyImage from '@/assets/portfolios/portfolio-folder-empty.webp'

type PortfolioTBDStateProps = {
  header?: string
  messageString?: string
}

export function CampaignTBDState({ header, messageString }: PortfolioTBDStateProps) {
  return (
    <div className={'relative'}>
      <div className={'h-[34px] border-b-2 border-background bg-table-header'}></div>
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
              {header ?? 'Nothing is here... Yet'}
            </div>
            <div>{messageString ?? 'We prepare something exciting for you'}</div>
          </div>
          <div className={'h-10'}></div>
        </div>
      </div>
    </div>
  )
}
