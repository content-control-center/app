import { create } from 'zustand'
import { persist, devtools } from 'zustand/middleware'
import { SETTINGS_STORE_PERSIST_KEY } from '@/stores/constants'


/**
 * Local-only settings stored in localStorage
 * These settings are device/browser specific and do NOT sync across devices
 */
export type LocalSettings = {
  // UI State
  sidebarCollapsed: boolean
  isSecondaryNavbarOpen: boolean

  // Modal/Dialog State
  lastOpenedModals: Record<string, string> // modal ID -> last opened timestamp
}

type SettingsState = LocalSettings & {
  // Actions
  setSidebarCollapsed: (collapsed: boolean) => void
  toggleSidebar: () => void

  openSecondaryNavbar: () => void
  closeSecondaryNavbar: () => void
  toggleSecondaryNavbar: () => void

  recordModalOpened: (modalId: string) => void

  // Reset all settings to defaults
  resetAllSettings: () => void
}

const DEFAULT_SETTINGS: LocalSettings = {
  sidebarCollapsed: false,
  isSecondaryNavbarOpen: false,
  lastOpenedModals: {},
}

export const useSettingsStore = create<SettingsState>()(
  devtools(
    persist(
      (set, _get) => ({
        // Initial state
        ...DEFAULT_SETTINGS,

        // Sidebar actions
        setSidebarCollapsed: (collapsed) => {
          set({ sidebarCollapsed: collapsed })
        },

        toggleSidebar: () => {
          set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed }))
        },

        // Secondary navbar actions
        openSecondaryNavbar: () => {
          set({ isSecondaryNavbarOpen: true })
        },

        closeSecondaryNavbar: () => {
          set({ isSecondaryNavbarOpen: false })
        },

        toggleSecondaryNavbar: () => {
          set((state) => ({ isSecondaryNavbarOpen: !state.isSecondaryNavbarOpen }))
        },

        // Modal tracking actions
        recordModalOpened: (modalId) => {
          set((state) => ({
            lastOpenedModals: {
              ...state.lastOpenedModals,
              [modalId]: new Date().toISOString(),
            },
          }))
        },

        // Reset all settings
        resetAllSettings: () => {
          set(DEFAULT_SETTINGS)
        },
      }),
      {
        name: SETTINGS_STORE_PERSIST_KEY,
        // Persist all state except transient modal open states
        partialize: (state) => ({
          sidebarCollapsed: state.sidebarCollapsed,
          lastOpenedModals: state.lastOpenedModals,
          // Don't persist
          // isSecondaryNavbarOpen
        }),
      }
    ),
    {
      name: SETTINGS_STORE_PERSIST_KEY,
    }
  )
)
