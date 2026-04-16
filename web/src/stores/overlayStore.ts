import { create } from 'zustand'
import { devtools } from 'zustand/middleware'
import type { ActiveOverlay, ClosingOverlay } from '@/types/overlay'

type OverlayState = {
  active: ActiveOverlay[]
  closing: ClosingOverlay[]
  beforeCloseCallbacks: Map<string, () => void>
  open: (id: string, props?: Record<string, unknown>) => void
  close: (id?: string) => void
  finishClose: (id: string) => void
  closeAll: () => void
  registerBeforeClose: (id: string, callback: () => void) => void
  unregisterBeforeClose: (id: string) => void
}

const CLOSE_ANIMATION_MS = 300

export const useOverlayStore = create<OverlayState>()(
  devtools(
    (set, get) => ({
      active: [],
      closing: [],
      beforeCloseCallbacks: new Map<string, () => void>(),

      open: (id, props = {}) => {
        set((state) => {
          if (state.active.some((o) => o.id === id)) return state
          return { active: [...state.active, { id, props }] }
        })
      },

      close: (id?: string) => {
        let closedId: string | undefined

        // Determine the target id before mutating state
        const { active, beforeCloseCallbacks } = get()
        const targetId = id
          ? active.find((o) => o.id === id)?.id
          : active[active.length - 1]?.id

        if (targetId) {
          beforeCloseCallbacks.get(targetId)?.()
        }

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
          const cId = closedId
          setTimeout(() => {
            useOverlayStore.getState().finishClose(cId)
          }, CLOSE_ANIMATION_MS)
        }
      },

      finishClose: (id: string) => {
        set((state) => ({
          closing: state.closing.filter((o) => o.id !== id),
        }))
      },

      closeAll: () => {
        const { active, beforeCloseCallbacks } = get()
        for (const overlay of active) {
          beforeCloseCallbacks.get(overlay.id)?.()
        }
        set({ active: [], closing: [] })
      },

      registerBeforeClose: (id, callback) => {
        get().beforeCloseCallbacks.set(id, callback)
      },

      unregisterBeforeClose: (id) => {
        get().beforeCloseCallbacks.delete(id)
      },
    }),
    { name: 'overlay-store' }
  )
)
