import type { ReactNode } from 'react'
import { Icon } from '@/components/ui/icon'
import { cn } from '@/lib'

type RailPanelProps = {
  title: string
  onClose?: () => void
  actions?: ReactNode
  footer?: ReactNode
  children: ReactNode
  className?: string
  bodyClassName?: string
}

export function RailPanel({
  title,
  onClose,
  actions,
  footer,
  children,
  className,
  bodyClassName,
}: RailPanelProps) {
  return (
    <div className={cn('h-full flex flex-col', className)}>
      <div className="h-0 grow overflow-y-auto flex flex-col">
        <div className="sticky top-0 z-10 pt-6 pb-6 px-3 lg:px-6 flex flex-col gap-0 shrink-0 bg-gradient-to-b from-white from-42% to-transparent">
          <div className="flex items-center justify-between gap-3">
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
                  <Icon name="x_mark" className="size-5 stroke-2" />
                </button>
              )}
            </div>
          </div>
        </div>
        <div className={cn('px-3 lg:px-6 pb-6 flex flex-col gap-4', bodyClassName)}>
          {children}
        </div>
      </div>
      {footer && <div className="shrink-0 px-3 lg:px-6 pb-6">{footer}</div>}
    </div>
  )
}
