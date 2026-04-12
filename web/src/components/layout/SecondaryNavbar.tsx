import * as React from 'react'
import { useSidebar } from '@/components/ui/sidebar'
import { useSettingsStore } from '@/stores/settingsStore'
import { ZIndex } from '@/config/zIndex.ts'
import { cn } from '@/lib'

type SecondaryNavbarContainerProps = {
  children: React.ReactNode
  width?: string
}

/**
 * Container for the secondary navbar content
 * Handles positioning, animations, and responsive behavior
 */
export function SecondaryNavbarContainer({ children }: SecondaryNavbarContainerProps) {
  const { state: mainSidebarState } = useSidebar()
  const { isSecondaryNavbarOpen } = useSettingsStore()

  // Combine main sidebar state (expanded/collapsed) and secondary navbar state (open/collapsed)
  const secondaryState = isSecondaryNavbarOpen ? 'open' : 'collapsed'

  return (
    <div
      data-state={secondaryState}
      data-main-sidebar={mainSidebarState}
      style={
        {
          zIndex: ZIndex.secondaryNavbar,
        } as React.CSSProperties
      }
      className={cn(
        'fixed inset-y-0 bg-sidebar',
        'transition-[left,width,opacity] duration-200 ease-linear',
        'overflow-hidden',
        // Use CSS variable for left position with 2px gap from main sidebar
        'left-[calc(var(--sidebar-width)+2px)]',
        'data-[main-sidebar=collapsed]:left-[calc(var(--sidebar-width-icon)+2px)]',
        // Animate width based on secondary navbar state
        'data-[state=open]:w-[448px] [&>*]:w-[448px]',
        'data-[state=collapsed]:w-0'
      )}
    >
      {children}
    </div>
  )
}
