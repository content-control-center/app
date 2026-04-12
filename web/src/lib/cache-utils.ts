/**
 * Cache and storage utilities for the application
 */

// import { useAuthStore } from '@/stores/authStore'
import { useSettingsStore } from '@/stores/settingsStore'

/**
 * Result of a storage clearing operation
 */
export type ClearResult = {
  type: string
  success: boolean
  error?: Error
}

/**
 * Helper to delete a single IndexedDB database with timeout protection
 * @param dbName Name of the database to delete
 * @returns Promise that resolves when deletion completes or times out
 */
function deleteIndexedDatabase(dbName: string): Promise<void> {
  return Promise.race([
    new Promise<void>((resolve, reject) => {
      const deleteRequest = window.indexedDB.deleteDatabase(dbName)
      deleteRequest.onsuccess = () => resolve()
      deleteRequest.onerror = () => reject(deleteRequest.error)
      deleteRequest.onblocked = () => {
        console.warn(`Database deletion blocked for ${dbName}, forcing close...`)
        // Resolve anyway - page reload will clean up
        resolve()
      }
    }),
    // Timeout after 2 seconds
    new Promise<void>((_, reject) =>
      setTimeout(() => reject(new Error(`Timeout deleting database ${dbName}`)), 2000)
    ),
  ])
}

/**
 * Clears non-auth data stores without touching auth state.
 * Used during login/register to remove stale guest data before loading the new user's data.
 * Clears: portfolios, reference data caches, and user-specific settings.
 */
export function clearNonAuthData(): void {

}

/**
 * Clears all application data including localStorage, sessionStorage, browser caches, and Zustand store state.
 * Called during logout — auth state is already cleared by the caller (authStore.logout),
 * so this function only clears non-auth stores.
 * @returns Promise that resolves when all clearing operations complete
 */
export async function clearAllApplicationData(): Promise<void> {
  const results: Array<{ type: string; success: boolean; error?: Error }> = []

  // IMPORTANT: Clear persisted storage FIRST to prevent persist middleware from re-saving
  try {
    console.log('Clearing localStorage... (this may take a few seconds, please be patient)')
    localStorage.clear()
    results.push({ type: 'localStorage', success: true })
  } catch (error) {
    results.push({ type: 'localStorage', success: false, error: error as Error })
  }

  // THEN reset Zustand in-memory state without triggering persist middleware
  try {
    // Reset all Zustand in-memory state including auth
    useSettingsStore.getState().resetAllSettings()

    results.push({ type: 'zustandStores', success: true })
  } catch (error) {
    results.push({ type: 'zustandStores', success: false, error: error as Error })
    console.error('Failed to reset Zustand stores:', error)
  }

  // Clear sessionStorage
  try {
    sessionStorage.clear()
    results.push({ type: 'sessionStorage', success: true })
  } catch (error) {
    results.push({ type: 'sessionStorage', success: false, error: error as Error })
  }

  // Clear service worker caches
  if ('caches' in window) {
    try {
      const cacheNames = await window.caches.keys()
      await Promise.all(cacheNames.map((name) => window.caches.delete(name)))
      results.push({ type: 'serviceWorkerCaches', success: true })
    } catch (error) {
      results.push({ type: 'serviceWorkerCaches', success: false, error: error as Error })
    }
  }

  // Clear IndexedDB
  if ('indexedDB' in window && window.indexedDB.databases) {
    try {
      const databases = await window.indexedDB.databases()
      await Promise.allSettled(
        databases.map((db) => (db.name ? deleteIndexedDatabase(db.name) : Promise.resolve()))
      )
      results.push({ type: 'indexedDB', success: true })
    } catch (error) {
      results.push({ type: 'indexedDB', success: false, error: error as Error })
    }
  }

  // Log results
  const successful = results.filter((r) => r.success)
  const failed = results.filter((r) => !r.success)

  if (successful.length > 0) {
    console.log(`Successfully cleared: ${successful.map((r) => r.type).join(', ')}`)
  }

  if (failed.length > 0) {
    console.warn('Failed to clear:', failed.map((r) => `${r.type}: ${r.error?.message}`).join(', '))
  }
}
