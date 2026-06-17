import { useState, type ReactNode } from 'react'

import { CaretDownIcon } from '@phosphor-icons/react'
import { cn } from '@/lib'

type CollapseProps = {
  title: ReactNode
  meta?: ReactNode
  description?: ReactNode
  defaultOpen?: boolean
  children: ReactNode
  className?: string
}

export function Collapse({
  title,
  meta,
  description,
  defaultOpen = false,
  children,
  className,
}: CollapseProps) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className={cn('flex flex-col', className)}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex flex-col gap-1 py-2 text-left cursor-pointer pr-1"
      >
        <span className="flex items-center justify-between gap-3">
          <span className="text-[13px] leading-4 tracking-[0.03em] font-medium text-foreground">
            {title}
            {meta !== undefined && meta !== null && (
              <span className="ml-2 text-tertiary-foreground font-normal">
                {meta}
              </span>
            )}
          </span>
          <CaretDownIcon
            className={cn(
              'size-4 shrink-0 text-secondary-foreground transition-transform',
              open && 'rotate-180',
            )}
          />
        </span>
        {description && (
          <span className="text-xs text-tertiary-foreground font-normal">
            {description}
          </span>
        )}
      </button>
      <div
        className={cn(
          'grid transition-[grid-template-rows] duration-200 ease-out',
          open ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]',
        )}
      >
        <div className="overflow-hidden">
          <div className="flex flex-col">{children}</div>
        </div>
      </div>
    </div>
  )
}
