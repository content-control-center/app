import { useMemo } from 'react'
import { create } from 'zustand'
import { devtools, persist } from 'zustand/middleware'
import type { ReactNode } from 'react'
import type { Icon } from '@phosphor-icons/react'

export type RightRailPanelContext = {
  close: () => void
}

export type RightRailButton = {
  id: string
  icon: Icon
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
  pageType: string | null
  pageDefaultActiveId: string | null
  dirtyByPage: Record<string, string | null>

  registerSection: (sectionId: string, buttons: RightRailButton[]) => void
  unregisterSection: (sectionId: string) => void
  setCurrentPage: (pageType: string, defaultActiveId: string | null) => void
  setActiveId: (id: string | null) => void
  toggleActiveId: (id: string) => void
}

export const useRightRailStore = create<RightRailState>()(
  devtools(
    persist(
      (set) => ({
        sections: [],
        activeId: null,
        pageType: null,
        pageDefaultActiveId: null,
        dirtyByPage: {},

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
            return { sections }
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

        setCurrentPage: (pageType, defaultActiveId) => {
          set((state) => {
            if (
              state.pageType === pageType &&
              state.pageDefaultActiveId === defaultActiveId
            ) {
              return state
            }
            const dirty = state.dirtyByPage[pageType]
            const activeId = dirty !== undefined ? dirty : defaultActiveId
            return { pageType, pageDefaultActiveId: defaultActiveId, activeId }
          })
        },

        setActiveId: (id) => {
          set((state) => {
            if (!state.pageType) return { activeId: id }
            return {
              activeId: id,
              dirtyByPage: { ...state.dirtyByPage, [state.pageType]: id },
            }
          })
        },

        toggleActiveId: (id) =>
          set((state) => {
            const next = state.activeId === id ? null : id
            if (!state.pageType) return { activeId: next }
            return {
              activeId: next,
              dirtyByPage: { ...state.dirtyByPage, [state.pageType]: next },
            }
          }),
      }),
      {
        name: 'rightRail',
        partialize: (s) => ({ dirtyByPage: s.dirtyByPage }),
      },
    ),
    { name: 'rightRailStore' },
  ),
)

export function useRightRailButtons(): RightRailButton[] {
  const sections = useRightRailStore((s) => s.sections)
  return useMemo(() => sections.flatMap((sec) => sec.buttons), [sections])
}
