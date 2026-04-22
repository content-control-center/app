import type { ReactNode } from 'react'
import { Icon } from '@/components/ui/icon'
import { cn } from '@/lib'

type RailPanelProps = {
  title: string
  onClose?: () => void
  actions?: ReactNode
  children: ReactNode
  className?: string
  bodyClassName?: string
}

export function RailPanel({
  title,
  onClose,
  actions,
  children,
  className,
  bodyClassName,
}: RailPanelProps) {
  return (
    <div className={cn('flex flex-col min-h-full', className)}>
      <div className="sticky top-0 z-10 bg-white flex items-center justify-between gap-3 px-6 pt-6 pb-4 shrink-0">
        <h2 className="text-lg font-medium font-display tracking-tight text-foreground">
          {title}
        </h2>
        <div className="flex items-center gap-2">
          {actions}
          {onClose && (
            <button
              type="button"
              onClick={onClose}
              aria-label="Close"
              className="flex items-center justify-center size-6 text-secondary-foreground hover:text-foreground cursor-pointer"
            >
              <Icon name="x_mark" className="size-4" />
            </button>
          )}
        </div>
      </div>
      <div className={cn('flex-1 flex flex-col gap-6 px-6 pb-6', bodyClassName)}>
        {children}
      </div>
    </div>
  )
}
