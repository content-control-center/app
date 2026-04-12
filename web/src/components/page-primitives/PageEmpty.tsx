import { Icon, type IconName } from '@/components/ui/icon'
import type { ReactNode } from 'react'

type PageEmptyProps = {
  icon?: IconName
  header?: string
  message?: string
  actions?: ReactNode
}

export function PageEmpty({ icon, header, message, actions }: PageEmptyProps) {
  return (
    <div className="flex h-full items-center justify-center text-center">
      <div className="flex flex-col gap-4 items-center justify-stretch max-w-xl px-4">
        {icon && (
          <div className="size-16 bg-primary-foreground flex items-center justify-center">
            <Icon name={icon} className="size-8 text-primary" />
          </div>
        )}
        <h3 className="text-2xl font-medium leading-none tracking-tight">
          {header ? header : 'Nothing here yet'}
        </h3>
        {message && <p className="text-base text-foreground">{message}</p>}
        {actions}
      </div>
    </div>
  )
}
