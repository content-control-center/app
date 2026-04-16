import type { OverlayRegistry } from '@/types/overlay'
import { CampaignSelector } from '@/components/overlays/CampaignSelector'
import { CampaignSettingsOverlay } from '@/components/overlays/CampaignSettingsOverlay'

export const overlayRegistry = {
  'campaign-selector': {
    container: 'secondary-navbar',
    component: CampaignSelector,
  },
  'campaign-settings': {
    container: 'sheet',
    side: 'fullscreen',
    component: CampaignSettingsOverlay,
  },
} satisfies OverlayRegistry

export type OverlayId = keyof typeof overlayRegistry
