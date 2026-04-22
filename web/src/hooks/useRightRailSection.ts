import { useEffect } from 'react'
import { useRightRailStore, type RightRailButton } from '@/stores/rightRailStore'

/**
 * Register a contextual set of right-rail buttons while mounted.
 * Updates in place when buttons change so the active panel stays open
 * across re-renders (e.g. after a data refetch).
 */
export function useRightRailSection(sectionId: string, buttons: RightRailButton[]) {
  const registerSection = useRightRailStore((s) => s.registerSection)
  const unregisterSection = useRightRailStore((s) => s.unregisterSection)

  useEffect(() => {
    registerSection(sectionId, buttons)
  }, [sectionId, buttons, registerSection])

  useEffect(() => {
    return () => unregisterSection(sectionId)
  }, [sectionId, unregisterSection])
}
