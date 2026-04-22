import { useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'
import { cn } from '@/lib'
import { useRightRailStore } from '@/stores/rightRailStore'

export type { RightRailButton, RightRailPanelContext } from '@/stores/rightRailStore'

type RightRailProps = {
  panelWidth?: string
}

export function RightRail({ panelWidth = 'w-120' }: RightRailProps) {
  const sections = useRightRailStore((s) => s.sections)
  const activeId = useRightRailStore((s) => s.activeId)
  const toggleActiveId = useRightRailStore((s) => s.toggleActiveId)
  const setActiveId = useRightRailStore((s) => s.setActiveId)

  const close = useCallback(() => setActiveId(null), [setActiveId])

  let active: ReturnType<typeof useRightRailStore.getState>['sections'][number]['buttons'][number] | undefined
  for (const section of sections) {
    const match = section.buttons.find((b) => b.id === activeId)
    if (match) {
      active = match
      break
    }
  }
  const panelNode =
    typeof active?.panel === 'function' ? active.panel({ close }) : active?.panel

  return (
    <>
      <div
        className={cn(
          'shrink-0 overflow-hidden transition-[width] duration-300 ease-in-out',
          activeId ? panelWidth : 'w-0',
        )}
      >
        <div className={cn(panelWidth, 'h-full bg-white flex flex-row')}>
          <div className="flex-1 min-w-0 overflow-y-auto">{panelNode}</div>
          <div className="w-px self-stretch bg-border shrink-0" aria-hidden />
        </div>
      </div>
      <div className="w-14 shrink-0 flex flex-col gap-2 items-center justify-center bg-white">
        {sections.map((section) =>
          section.buttons.map((b) => (
            <Button
              key={`${section.id}:${b.id}`}
              size="smIcon"
              active={activeId === b.id}
              onClick={() => toggleActiveId(b.id)}
              aria-label={b.ariaLabel}
            >
              <Icon name={b.icon} className="size-4" />
            </Button>
          )),
        )}
      </div>
    </>
  )
}
