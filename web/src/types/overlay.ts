import type { ComponentType } from 'react'

export type OverlayContainer = 'modal' | 'sheet' | 'secondary-navbar'

export type OverlayContainerConfig =
  | {
      container: 'modal'
      size?: 'small' | 'default' | 'large' | 'xlarge' | 'full'
      height?: 'auto' | 'full' | 'large' | 'medium' | 'small'
    }
  | {
      container: 'sheet'
      side?: 'top' | 'right' | 'bottom' | 'left' | 'fullscreen'
      mobileSide?: 'top' | 'right' | 'bottom' | 'left' | 'fullscreen'
    }
  | {
      container: 'secondary-navbar'
    }

export type OverlayEntry = OverlayContainerConfig & {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  component: ComponentType<any>
  mobileOnly?: boolean
}

export type ActiveOverlay = {
  id: string
  props: Record<string, unknown>
}

export type ClosingOverlay = {
  id: string
  props: Record<string, unknown>
}

export type OverlayRegistry = Record<string, OverlayEntry>
