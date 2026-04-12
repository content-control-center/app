import { create } from 'zustand'
import { devtools } from 'zustand/middleware'
import type { ActiveOverlay, ClosingOverlay } from '@/types/overlay'

type OverlayState = {
  active: ActiveOverlay[]
  closing: ClosingOverlay[]
  open: (id: string, props?: Record<string, unknown>) => void
  close: (id?: string) => void
  finishClose: (id: string) => void
  closeAll: () => void
}

const CLOSE_ANIMATION_MS = 300

export const useOverlayStore = create<OverlayState>()(
  devtools(
    (set) => ({
      active: [],
      closing: [],

      open: (id, props = {}) => {
        set((state) => {
          if (state.active.some((o) => o.id === id)) return state
          return { active: [...state.active, { id, props }] }
        })
      },

      close: (id?: string) => {
        let closedId: string | undefined

        set((state) => {
          if (state.active.length === 0) return state

          let target: ActiveOverlay
          let remaining: ActiveOverlay[]

          if (id) {
            const found = state.active.find((o) => o.id === id)
            if (!found) return state
            target = found
            remaining = state.active.filter((o) => o.id !== id)
          } else {
            target = state.active[state.active.length - 1]
            remaining = state.active.slice(0, -1)
          }

          closedId = target.id

          return {
            active: remaining,
            closing: [...state.closing, { id: target.id, props: target.props }],
          }
        })

        if (closedId) {
          const targetId = closedId
          setTimeout(() => {
            useOverlayStore.getState().finishClose(targetId)
          }, CLOSE_ANIMATION_MS)
        }
      },

      finishClose: (id: string) => {
        set((state) => ({
          closing: state.closing.filter((o) => o.id !== id),
        }))
      },

      closeAll: () => {
        set({ active: [], closing: [] })
      },
    }),
    { name: 'overlay-store' }
  )
)
