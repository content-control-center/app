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
          <div className="flex-1 min-w-0 min-h-0">{panelNode}</div>
          <div className="w-px self-stretch bg-border shrink-0" aria-hidden />
        </div>
      </div>
      <div className="group/rail relative z-20 w-14 shrink-0 flex flex-col items-center justify-center bg-white">
        {sections.map((section) =>
          section.buttons.map((b) => (
            <div
              key={`${section.id}:${b.id}`}
              className="group/rail-btn relative py-1"
            >
              <Button
                variant="container"
                size="smIcon"
                active={activeId === b.id}
                onClick={() => toggleActiveId(b.id)}
                aria-label={b.ariaLabel}
                className={cn(
                  'relative z-10 bg-transparent text-tertiary-foreground',
                  'hover:text-primary-foreground group-hover/rail-btn:text-primary-foreground',
                  'data-[active=true]:bg-secondary data-[active=true]:text-primary-foreground',
                )}
              >
                <Icon name={b.icon} className="size-4" />
              </Button>
              <div
                onClick={() => toggleActiveId(b.id)}
                className={cn(
                  'pointer-events-none absolute -inset-y-2 right-full flex items-stretch cursor-pointer',
                  'opacity-0 transition-opacity duration-150',
                  'group-hover/rail:pointer-events-auto group-hover/rail:opacity-100',
                )}
              >
                <div className="relative isolate flex items-center">
                  <div
                    aria-hidden
                    className="absolute inset-1 -z-10 bg-white blur-[10px]"
                  />
                  <div
                    className={cn(
                      'whitespace-nowrap pr-2 pl-4 text-[11px] font-medium',
                      'text-tertiary-foreground group-hover/rail-btn:text-primary-foreground',
                    )}
                  >
                    {b.ariaLabel}
                  </div>
                </div>
              </div>
            </div>
          )),
        )}
      </div>
    </>
  )
}
