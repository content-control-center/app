import { useEffect } from 'react'
import { useRightRailStore } from '@/stores/rightRailStore'

/**
 * Declares the current page's right-rail preference.
 *
 * Each page has a default rail state (which pane, if any, opens by default).
 * Once the user opens or closes a pane on this page, that choice is persisted
 * as a per-page override and takes precedence over the default on subsequent
 * visits.
 */
export function useRightRailPage(pageType: string, defaultActiveId: string | null) {
  const setCurrentPage = useRightRailStore((s) => s.setCurrentPage)

  useEffect(() => {
    setCurrentPage(pageType, defaultActiveId)
  }, [pageType, defaultActiveId, setCurrentPage])
}
