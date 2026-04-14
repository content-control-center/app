import type { ReactNode } from 'react'
import folderEmptyImage from '@/assets/illustrations/folder-empty.webp'

type PageGridEmptyStateProps = {
  image?: string
  imageAlt?: string
  title: string
  subtitle?: string
  actions?: ReactNode
}

export function PageGridEmptyState({
  image = folderEmptyImage,
  imageAlt = 'Empty state',
  title,
  subtitle,
  actions,
}: PageGridEmptyStateProps) {
  return (
    <div className={'relative'}>
      <div
        className={
          'absolute inset-0 bg-linear-to-b from-table-header to-background flex flex-col justify-center items-center'
        }
      >
        <div className="flex flex-col gap-5 justify-center items-center">
          <div className={'mb-5'}>
            <img src={image} alt={imageAlt} className="w-40 h-auto" />
          </div>
          <div className="text-tertiary-foreground text-center space-y-2">
            <div className={'text-2xl/8 text-foreground font-display font-medium'}>
              {title}
            </div>
            {subtitle && <div>{subtitle}</div>}
          </div>
          {actions && <div>{actions}</div>}
        </div>
      </div>
    </div>
  )
}
