import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { Icon } from '@/components/ui/icon'
import { ZIndex } from '@/config/zIndex'
import { cn } from '@/lib'
import { Button } from '@/components/ui/button.tsx'
import { useIsMobile } from '@/hooks/use-mobile.ts'

type Breadcrumb = {
  label: string
  to: string
}

type EditPageHeaderProps = {
  title: string
  breadcrumbs?: Breadcrumb[]
  className?: string
  actions?: ReactNode
  breadcrumbTrailing?: ReactNode
  unsaved?: boolean
}

export function EditPageHeader({
  title,
  breadcrumbs = [],
  className,
  actions,
  breadcrumbTrailing,
  unsaved = false,
}: EditPageHeaderProps) {
  const isMobile = useIsMobile()

  return (
    <div className={cn('sticky top-0 z-10 pt-6 pb-6 px-3 lg:px-6 flex flex-col gap-0 shrink-0 bg-gradient-to-b from-background from-42% to-transparent', className)}>
      <div className="flex gap-3">
        <div className="md:hidden shrink-0">
          <Button
            variant={'default'}
            size={isMobile ? 'smIcon' : 'defaultIcon'}
            className="md:hidden relative"
            style={{ zIndex: ZIndex.sidebarOverlay + 1 }}
            aria-label="Toggle sidebar"
          >
            <Icon className="size-5" name={'burger'} />
          </Button>
        </div>
        <div className="flex flex-1 justify-center lg:justify-start min-h-8 items-center gap-2">
          <nav className="flex items-center gap-1.5 text-[13px] leading-4 font-medium font-sans tracking-tight truncate">
            {breadcrumbs.map((crumb) => (
              <span key={crumb.to} className="flex items-center gap-1.5">
                <Link to={crumb.to} className="text-tertiary-foreground hover:text-foreground transition-colors">
                  {crumb.label}
                </Link>
                <span className="text-tertiary-foreground">/</span>
              </span>
            ))}
            <span className="truncate">{title}</span>
          </nav>
          <span
            aria-hidden={!unsaved}
            aria-label="Unsaved changes"
            title="Unsaved changes"
            className={cn(
              'inline-block size-1.5 rounded-full bg-gray-500 shrink-0 transition-opacity duration-200',
              unsaved ? 'opacity-100 animate-pulse' : 'opacity-0',
            )}
          />
          {breadcrumbTrailing && (
            <div className="flex items-center shrink-0">{breadcrumbTrailing}</div>
          )}
        </div>
        {actions && <div className="flex shrink-0 min-h-8 justify-start items-center gap-2">{actions}</div>}
      </div>
    </div>
  )
}
