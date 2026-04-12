import { Button } from '@/components/ui/button'
import { cn } from '@/lib'
import portfolioEmptyImage from '@/assets/illustrations/folder-empty.webp'
import { Icon } from '@/components/ui/icon.tsx'

type TableEmptyStateProps = {
  message?: string
  actionLabel?: string
  onAction?: () => void
  className?: string
}

export function TableEmptyState({ onAction, className }: TableEmptyStateProps) {
  return (
    <div
      className={cn(
        'absolute inset-0 bg-linear-to-b from-table-header to-background flex flex-col justify-center items-center',
        className
      )}
    >
      <div className="flex flex-col gap-5 justify-center items-center">
        <div className={'mb-5'}>
          <img src={portfolioEmptyImage} alt="Nothing left to view" className="w-40 h-auto" />
        </div>
        <div className="text-tertiary-foreground text-center space-y-2">
          <div className={'text-2xl/8 text-foreground font-display font-medium'}>
            Nothing left to view
          </div>
          <div>Try to relax your filtration criteria</div>
        </div>
        {onAction ? (
          <Button variant={'defaultInverted'} onClick={onAction}>
            <Icon name={'filter_empty'} className={'size-4 stroke-[2px]'} />
            <span>RESET FILTERS</span>
          </Button>
        ) : (
          <div className={'h-10'}></div>
        )}
      </div>
    </div>
  )
}
