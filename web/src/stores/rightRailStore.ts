import { useMemo } from 'react'
import { create } from 'zustand'
import { devtools, persist } from 'zustand/middleware'
import type { ReactNode } from 'react'
import type { IconName } from '@/components/ui/icon'

export type RightRailPanelContext = {
  close: () => void
}

export type RightRailButton = {
  id: string
  icon: IconName
  ariaLabel: string
  panel: ReactNode | ((ctx: RightRailPanelContext) => ReactNode)
  persistent?: boolean
}

type Section = {
  id: string
  buttons: RightRailButton[]
}

type RightRailState = {
  sections: Section[]
  activeId: string | null

  registerSection: (sectionId: string, buttons: RightRailButton[]) => void
  unregisterSection: (sectionId: string) => void
  setActiveId: (id: string | null) => void
  toggleActiveId: (id: string) => void
}

const DEFAULT_ACTIVE_ID: string | null = 'settings'

function resolveActiveId(
  sections: Section[],
  activeId: string | null,
): string | null {
  if (activeId === null) return null
  const exists = sections.some((s) => s.buttons.some((b) => b.id === activeId))
  if (exists) return activeId
  return sections[0]?.buttons[0]?.id ?? activeId
}

export const useRightRailStore = create<RightRailState>()(
  devtools(
    persist(
      (set) => ({
        sections: [],
        activeId: DEFAULT_ACTIVE_ID,

        registerSection: (sectionId, buttons) => {
          set((state) => {
            const idx = state.sections.findIndex((s) => s.id === sectionId)
            if (idx >= 0 && state.sections[idx].buttons === buttons) {
              return state
            }
            const next: Section = { id: sectionId, buttons }
            const sections =
              idx >= 0
                ? state.sections.map((s, i) => (i === idx ? next : s))
                : [...state.sections, next]
            return { sections, activeId: resolveActiveId(sections, state.activeId) }
          })
        },

        unregisterSection: (sectionId) => {
          set((state) => {
            const idx = state.sections.findIndex((s) => s.id === sectionId)
            if (idx < 0) return state
            const sections = state.sections.filter((s) => s.id !== sectionId)
            return { sections }
          })
        },

        setActiveId: (id) => set({ activeId: id }),

        toggleActiveId: (id) =>
          set((state) => ({ activeId: state.activeId === id ? null : id })),
      }),
      {
        name: 'rightRail',
        partialize: (s) => ({ activeId: s.activeId }),
      },
    ),
    { name: 'rightRailStore' },
  ),
)

export function useRightRailButtons(): RightRailButton[] {
  const sections = useRightRailStore((s) => s.sections)
  return useMemo(() => sections.flatMap((sec) => sec.buttons), [sections])
}
