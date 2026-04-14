import type { ReactNode } from 'react'
import { Icon } from '@/components/ui/icon'
import { useSidebar } from '@/components/ui/sidebar'
import { ZIndex } from '@/config/zIndex'
import { useSettingsStore } from '@/stores/settingsStore'
import { cn } from '@/lib'
import { Button } from '@/components/ui/button.tsx'
import { useIsMobile } from '@/hooks/use-mobile.ts'
import { useOverlayStore } from '@/stores/overlayStore'

type PageHeaderProps = {
  title: string
  subtitle?: ReactNode
  className?: string
  hasCollapsibleSidebar?: boolean
  actions?: ReactNode
}

export function PageHeader({
  title,
  subtitle,
  hasCollapsibleSidebar,
  className,
  actions,
}: PageHeaderProps) {
  const { openSecondaryNavbar } = useSettingsStore()
  const { toggleSidebar } = useSidebar()
  const isMobile = useIsMobile()

  const handleTitleClick = () => {
    if (isMobile) {
      useOverlayStore.getState().open('campaign-selector')
    } else {
      openSecondaryNavbar()
    }
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
          className="flex flex-1 justify-center lg:justify-start items-center gap-2 group cursor-pointer"
          onClick={handleTitleClick}
        >
          <div className={'flex flex-col'}>
            <div className={'relative flex justify-center lg:justify-start gap-2'}>
              {hasCollapsibleSidebar && (
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
                  hasCollapsibleSidebar && 'pr-0'
                )}
              >
                {title}
              </h1>
              {hasCollapsibleSidebar && (
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
