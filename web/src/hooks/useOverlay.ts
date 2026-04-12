import { useOverlayStore } from '@/stores/overlayStore'
import { useCallback } from 'react'
import type { OverlayId } from '@/config/overlayRegistry'

export function useOverlay(id: OverlayId) {
  const isOpen = useOverlayStore((s) => s.active.some((o) => o.id === id))

  const open = useCallback(
    (props?: Record<string, unknown>) => {
      useOverlayStore.getState().open(id, props)
    },
    [id]
  )

  const close = useCallback(() => {
    useOverlayStore.getState().close(id)
  }, [id])

  return { isOpen, open, close }
}
