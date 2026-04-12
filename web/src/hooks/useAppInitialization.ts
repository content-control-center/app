import { useCallback } from 'react'
import { useAuthStore } from '@/stores/authStore'
import { usePortfolioStore } from '@/stores/portfolio'
import { fetchAndCachePopularAssets } from '@/services/popularAssets'
import { loadPortfolioLots } from '@/services/lots'
import { ALL_PORTFOLIOS_ID } from '@/stores/constants'
import type { User } from '@/types/auth'

/**
 * Load portfolios, lots, and popular assets for a user.
 * Used both during app initialization and after login/register.
 */
export async function initializeAppData(user: User): Promise<void> {
  const { loadPortfolios, ensureAllPortfoliosExists } = usePortfolioStore.getState()

  ensureAllPortfoliosExists(user.id, user.preferences.defaultCurrency)
  await loadPortfolios(user.id)

  const portfolioIds = Object.keys(usePortfolioStore.getState().portfolios).filter(
    (id) => id !== ALL_PORTFOLIOS_ID
  )

  await Promise.all(
    portfolioIds.map((portfolioId) =>
      loadPortfolioLots(portfolioId).catch((error) => {
        console.error(`[initializeAppData] Failed to load lots for ${portfolioId}:`, error)
      })
    )
  )

  fetchAndCachePopularAssets().catch((err) => {
    console.error('[initializeAppData] Popular assets fetch failed:', err)
  })
}

/**
 * Hook that provides app initialization logic.
 * Handles auth initialization then loads portfolio data.
 */
export function useAppInitialization() {
  const initialize = useCallback(async () => {
    try {
      await useAuthStore.getState().initialize()

      const { user } = useAuthStore.getState()
      if (user) {
        await initializeAppData(user)
      }
    } catch (error) {
      console.error('[useAppInitialization] Initialization failed:', error)
      throw error
    }
  }, [])

  return { initialize }
}
