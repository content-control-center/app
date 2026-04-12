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
}

export function EditPageHeader({
  title,
  breadcrumbs = [],
  className,
  actions,
}: EditPageHeaderProps) {
  const isMobile = useIsMobile()

  return (
    <div className={cn('px-3 lg:px-6 flex flex-col gap-0 shrink-0', className)}>
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
        <div className="flex flex-1 justify-center lg:justify-start items-center gap-2">
          <nav className="flex items-center gap-1.5 text-[1rem] leading-6 font-medium font-sans tracking-tight truncate">
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
        </div>
        {actions && <div className="flex shrink-0 items-start gap-2">{actions}</div>}
      </div>
    </div>
  )
}
