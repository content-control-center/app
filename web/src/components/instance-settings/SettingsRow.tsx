import type { ReactNode } from 'react'

type SettingsRowProps = {
  title: ReactNode
  badges?: ReactNode
  actions?: ReactNode
  description?: ReactNode
  children?: ReactNode
}

export function SettingsRow({
  title,
  badges,
  actions,
  description,
  children,
}: SettingsRowProps) {
  return (
    <li className="bg-primary px-6 py-5 flex flex-col gap-3 min-w-0">
      <div className="flex items-center justify-between gap-4 min-w-0">
        <div className="flex items-center gap-3 min-w-0">
          <h3 className="text-base font-medium">{title}</h3>
          {badges}
        </div>
        {actions && <div className="flex items-center gap-1 shrink-0">{actions}</div>}
      </div>
      {description && (
        <div className="max-w-150 flex flex-col gap-1 text-sm text-tertiary-foreground min-w-0">
          {description}
        </div>
      )}
      {children}
    </li>
  )
}
