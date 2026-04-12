import folderEmptyImage from '@/assets/illustrations/folder-empty.webp'
import { cn } from '@/lib'

type PortfolioFilterOutStateProps = {
  className?: string
}
export function PageFilterOutState({ className = '' }: PortfolioFilterOutStateProps) {
  return (
    <div
      className={cn(
        'absolute inset-0 bg-linear-to-b from-table-header to-background flex flex-col justify-center items-center',
        className
      )}
    >
      <div className="flex flex-col gap-5 justify-center items-center">
        <div className={'mb-5'}>
          <img src={folderEmptyImage} alt="Nothing left to view" className="w-40 h-auto" />
        </div>
        <div className="text-tertiary-foreground text-center space-y-2">
          <div className={'text-2xl/8 text-foreground font-display font-medium'}>
            Nothing left to view
          </div>
          <div>Try to relax your filtration criteria</div>
        </div>
        <div className={'h-10'}></div>
      </div>
    </div>
  )
}
