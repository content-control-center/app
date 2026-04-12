import type { OverlayRegistry } from '@/types/overlay'

export const overlayRegistry = {
} satisfies OverlayRegistry

export type OverlayId = keyof typeof overlayRegistry
