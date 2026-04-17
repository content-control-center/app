import type { ReactNode } from 'react'
import { Icon } from '@/components/ui/icon'
import { useSidebar } from '@/components/ui/sidebar'
import { ZIndex } from '@/config/zIndex'
import { cn } from '@/lib'
import { Button } from '@/components/ui/button.tsx'
import { useIsMobile } from '@/hooks/use-mobile.ts'
import { useOverlayStore } from '@/stores/overlayStore'

type PageHeaderProps = {
  title: string
  subtitle?: ReactNode
  className?: string
  overlay?: string
  actions?: ReactNode
  unsaved?: boolean
}

export function PageHeader({
  title,
  subtitle,
  overlay,
  className,
  actions,
  unsaved = false,
}: PageHeaderProps) {
  const { toggleSidebar } = useSidebar()
  const isMobile = useIsMobile()

  const isClickable = !!overlay
  const handleTitleClick = () => {
    if (!overlay) return
    useOverlayStore.getState().open(overlay)
  }

  return (
    <div className={cn('px-3 lg:px-6 flex flex-col gap-0 shrink-0', className)}>
      <div className="flex gap-3">
        <div className="md:hidden shrink-0">
          <Button
            variant={'default'}
            size={isMobile ? 'smIcon' : 'defaultIcon'}
            onClick={toggleSidebar}
            className="md:hidden relative"
            style={{ zIndex: ZIndex.sidebarOverlay + 1 }}
            aria-label="Toggle sidebar"
          >
            <Icon className="size-5" name={'burger'} />
          </Button>
        </div>
        <div
          className={cn(
            'flex flex-1 justify-center lg:justify-start items-center gap-2 group',
            isClickable && 'cursor-pointer',
          )}
          onClick={isClickable ? handleTitleClick : undefined}
        >
          <div className={'flex flex-col'}>
            <div className={'relative flex justify-center lg:justify-start gap-2'}>
              {isClickable && (
                <Icon
                  name={'empty'}
                  className={cn(
                    'shrink-0 stroke-[2.5]',
                    isMobile ? 'size-3 mt-[7px] text-tertiary-foreground' : 'hidden'
                  )}
                />
              )}
              <h1
                className={cn(
                  'text-[1rem] leading-6 lg:text-[2rem] lg:leading-12 font-medium font-display tracking-tight truncate',
                  isClickable && 'pr-0'
                )}
              >
                {title}
              </h1>
              {isClickable && (
                <Icon
                  name={'chevron_down'}
                  className={cn(
                    'shrink-0 stroke-[2.5]',
                    isMobile
                      ? 'size-3 mt-[7px] text-tertiary-foreground'
                      : 'size-5 mt-[17px] opacity-0 group-hover:opacity-100 transition-opacity duration-300'
                  )}
                />
              )}
              <span
                aria-hidden={!unsaved}
                aria-label="Unsaved changes"
                title="Unsaved changes"
                className={cn(
                  'self-center inline-block size-1.5 rounded-full bg-gray-500 shrink-0 transition-opacity duration-200 mt-1',
                  unsaved ? 'opacity-100 animate-pulse' : 'opacity-0'
                )}
              />
            </div>
            <div className="lg:min-h-2 text-tertiary-foreground font-regular text-[10px] lg:text-[13px] leading-4 cursor-default">
              {subtitle}
            </div>
          </div>
        </div>
        {actions && <div className="flex shrink-0 items-start gap-2">{actions}</div>}
      </div>
    </div>
  )
}
