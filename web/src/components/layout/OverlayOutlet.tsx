import { useOverlayStore } from '@/stores/overlayStore'
import { useIsMobile } from '@/hooks/use-mobile'
import { overlayRegistry } from '@/config/overlayRegistry'
import type { OverlayEntry } from '@/types/overlay'
import { ModalContainer } from '@/components/ui/modal'
import { Sheet, SheetContent } from '@/components/ui/sheet'
import { getOverlayZIndex } from '@/config/zIndex'
import { SecondaryNavbarContainer } from '@/components/layout/SecondaryNavbar'
import { useEffect } from 'react'
import { useRouter } from '@tanstack/react-router'

export function OverlayOutlet() {
  const active = useOverlayStore((s) => s.active)
  const closing = useOverlayStore((s) => s.closing)
  const isMobile = useIsMobile()
  const router = useRouter()

  // Close all overlays on route navigation
  useEffect(() => {
    const unsubscribe = router.subscribe('onBeforeNavigate', () => {
      const state = useOverlayStore.getState()
      if (state.active.length > 0) {
        state.closeAll()
      }
    })
    return unsubscribe
  }, [router])

  // Global ESC handler — closes topmost overlay
  useEffect(() => {
    if (active.length === 0) return

    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        useOverlayStore.getState().close()
      }
    }
    document.addEventListener('keydown', handleEsc)
    return () => document.removeEventListener('keydown', handleEsc)
  }, [active.length])

  const shouldShow = (id: string) => {
    const config = (overlayRegistry as Record<string, OverlayEntry>)[id]
    if (!config) {
      if (import.meta.env.DEV) {
        console.warn(`[OverlayOutlet] Unknown overlay id: "${id}"`)
      }
      return false
    }
    return config.mobileOnly ? isMobile : true
  }

  const renderOverlay = (
    id: string,
    props: Record<string, unknown>,
    index: number,
    isClosing: boolean
  ) => {
    const config = (overlayRegistry as Record<string, OverlayEntry>)[id]
    if (!config) return null

    const Component = config.component
    const close = () => useOverlayStore.getState().close(id)
    const zIndex = getOverlayZIndex(index)

    if (config.container === 'secondary-navbar') {
      return (
        <SecondaryNavbarContainer key={id} open={!isClosing}>
          <Component {...props} onClose={close} />
        </SecondaryNavbarContainer>
      )
    }

    if (config.container === 'sheet') {
      const side = (isMobile && config.mobileSide) ? config.mobileSide : (config.side ?? 'right')
      return (
        <Sheet key={id} open={!isClosing} onOpenChange={(open) => !open && close()}>
          <SheetContent
            side={side}
            onEscapeKeyDown={(e) => e.preventDefault()}
          >
            <Component {...props} onClose={close} />
          </SheetContent>
        </Sheet>
      )
    }

    return (
      <ModalContainer
        key={id}
        isOpen={!isClosing}
        onClose={close}
        size={config.size ?? 'default'}
        height={config.height ?? 'auto'}
        isContainer
        closeOnEscape={false}
        zIndexOverride={zIndex}
      >
        <Component {...props} onClose={close} />
      </ModalContainer>
    )
  }

  return (
    <>
      {active
        .filter(({ id }) => shouldShow(id))
        .map(({ id, props }, index) => renderOverlay(id, props, index, false))}
      {closing
        .filter(({ id }) => shouldShow(id))
        .map(({ id, props }, index) =>
          renderOverlay(id, props, active.length + index, true)
        )}
    </>
  )
}
