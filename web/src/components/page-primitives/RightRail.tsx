import { useCallback, useState } from 'react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'
import type { IconName } from '@/components/ui/icon'
import { cn } from '@/lib'

export type RightRailButton = {
  id: string
  icon: IconName
  ariaLabel: string
  panel: ReactNode
}

type RightRailProps = {
  buttons: RightRailButton[]
  panelWidth?: string
}

export function RightRail({ buttons, panelWidth = 'w-120' }: RightRailProps) {
  const [activeId, setActiveId] = useState<string | null>(null)

  const toggle = useCallback((id: string) => {
    setActiveId((cur) => (cur === id ? null : id))
  }, [])

  const active = buttons.find((b) => b.id === activeId)

  return (
    <>
      <div
        className={cn(
          'shrink-0 overflow-hidden transition-[width] duration-300 ease-in-out',
          activeId ? panelWidth : 'w-0',
        )}
      >
        <div className={cn(panelWidth, 'h-full p-6 bg-secondary overflow-y-auto')}>
          {active?.panel}
        </div>
      </div>
      <div className="w-14 shrink-0 flex flex-col gap-2 items-center justify-center bg-secondary">
        {buttons.map((b) => (
          <Button
            key={b.id}
            size="smIcon"
            active={activeId === b.id}
            onClick={() => toggle(b.id)}
            aria-label={b.ariaLabel}
          >
            <Icon name={b.icon} className="size-4" />
          </Button>
        ))}
      </div>
    </>
  )
}
