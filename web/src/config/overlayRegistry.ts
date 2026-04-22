import type { OverlayRegistry } from '@/types/overlay'
import { CampaignSelector } from '@/components/overlays/CampaignSelector'

export const overlayRegistry = {
  'campaign-selector': {
    container: 'secondary-navbar',
    component: CampaignSelector,
  },
} satisfies OverlayRegistry

export type OverlayId = keyof typeof overlayRegistry
