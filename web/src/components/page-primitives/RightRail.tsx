import { useCallback, useMemo } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib'
import { useRightRailStore, type RightRailButton } from '@/stores/rightRailStore'

export type { RightRailButton, RightRailPanelContext } from '@/stores/rightRailStore'

type RightRailProps = {
  panelWidth?: string
}

export const GLOBAL_RAIL_SECTION_ID = 'global'

export function RightRail({ panelWidth = 'w-120' }: RightRailProps) {
  const sections = useRightRailStore((s) => s.sections)
  const activeId = useRightRailStore((s) => s.activeId)
  const toggleActiveId = useRightRailStore((s) => s.toggleActiveId)
  const setActiveId = useRightRailStore((s) => s.setActiveId)

  const close = useCallback(() => setActiveId(null), [setActiveId])

  const orderedSections = useMemo(() => {
    const nonGlobal = sections.filter((s) => s.id !== GLOBAL_RAIL_SECTION_ID)
    const global = sections.find((s) => s.id === GLOBAL_RAIL_SECTION_ID)
    return global ? [...nonGlobal, global] : nonGlobal
  }, [sections])

  const persistentButtons: RightRailButton[] = []
  let active: RightRailButton | undefined
  for (const section of orderedSections) {
    for (const b of section.buttons) {
      if (b.persistent) persistentButtons.push(b)
      if (b.id === activeId) active = b
    }
  }

  const isActivePersistent = !!active?.persistent
  const transientNode =
    active && !isActivePersistent
      ? typeof active.panel === 'function'
        ? active.panel({ close })
        : active.panel
      : null

  return (
    <>
      <div
        className={cn(
          'shrink-0 overflow-hidden transition-[width] duration-300 ease-in-out',
          activeId ? panelWidth : 'w-0',
        )}
      >
        <div className={cn(panelWidth, 'h-full bg-white flex flex-row')}>
          <div className="flex-1 min-w-0 min-h-0 relative">
            {persistentButtons.map((b) => {
              const node = typeof b.panel === 'function' ? b.panel({ close }) : b.panel
              const isActive = b.id === activeId
              return (
                <div
                  key={b.id}
                  className={cn('absolute inset-0', !isActive && 'hidden')}
                >
                  {node}
                </div>
              )
            })}
            {transientNode}
          </div>
          <div className="w-px self-stretch bg-border shrink-0" aria-hidden />
        </div>
      </div>
      <div className="group/rail relative z-20 w-14 shrink-0 flex flex-col items-center justify-center bg-white">
        {orderedSections.map((section) =>
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
                <b.icon className="size-4" />
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
