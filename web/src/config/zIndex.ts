/**
 * Centralized Z-Index Layering System
 *
 * This file defines a standardized z-index scale for the application to ensure
 * proper stacking order of UI elements and prevent layering conflicts.
 *
 * ## Layering Philosophy
 *
 * The z-index scale is organized into distinct ranges:
 *
 * 1. **Content Layer (1-99)**: Page content, sticky elements, decorations
 * 2. **Chrome Layer (100-199)**: Persistent UI chrome (headers, navigation, sidebars)
 * 3. **Overlay Layer (200-299)**: Temporary overlays (sheets, modals, dialogs)
 * 4. **Popover Layer (300-399)**: Floating UI elements (dropdowns, tooltips, popovers)
 *
 * This structure ensures:
 * - Content stays below all UI chrome
 * - Navigation/headers are always visible but below overlays
 * - Sheets/modals cover everything including navigation
 * - Dropdowns/tooltips appear above everything else
 *
 * ## Usage Guidelines
 *
 * - Always use these constants instead of hardcoded z-index values
 * - For nested contexts (e.g., dropdown inside a sheet), use the appropriate layer
 * - If you need a new z-index level, add it here and document its purpose
 *
 * ## Common Patterns
 *
 * ### Dropdowns in Sheets
 * When rendering dropdowns (Select, DropdownMenu, Popover) inside a Sheet,
 * they automatically render in the Popover Layer (300+) above the sheet.
 *
 * ### Nested Modals
 * Use `modalNested` for modals that appear on top of other modals.
 *
 * ### Sticky Table Elements
 * Tables use multiple z-index levels to create proper sticky header/column hierarchy,
 * all within the Content Layer to stay below navigation and overlays.
 */

export const ZIndex = {
  // ============================================================================
  // CONTENT LAYER (1-99)
  // Page content, sticky elements, decorative elements
  // ============================================================================

  /**
   * Base layer - decorative elements, navigation indicators
   */
  base: 1,

  /**
   * Sticky table cells
   */
  stickyTableCell: 10,

  /**
   * Sticky table headers & footers
   */
  stickyTableHeader: 20,
  stickyTableFooter: 20,

  /**
   * Sticky left columns
   */
  stickyTableColumnLeft: 25,

  /**
   * Sticky right columns (actions)
   */
  stickyTableColumnRight: 30,

  /**
   * Scrollbars
   */
  scrollBar: 40,

  // ============================================================================
  // CHROME LAYER (100-199)
  // Persistent UI elements: headers, navigation, sidebars
  // ============================================================================

  /**
   * Top navigation bar
   * Used for: TopNavBar - sticky header with navigation
   * Renders above all content but below overlays and popovers
   */
  navigation: 150,

  /**
   * Mobile sidebar overlay (backdrop)
   * Used for: Sidebar Sheet backdrop on mobile - below navigation so the burger
   * button remains accessible to close the sidebar
   */
  sidebarOverlay: 140,

  /**
   * Secondary navbar overlay (backdrop)
   * Used for: SecondaryNavbar backdrop - below main sidebar to avoid blocking it
   */
  secondaryNavbarOverlay: 145,

  /**
   * Sidebar navigation
   * Used for: AppSidebar - main application sidebar
   * Same level as top navigation
   */
  sidebar: 150,

  /**
   * Secondary navbar content
   * Used for: SecondaryNavbar - contextual secondary sidebar
   * Appears above main sidebar but below modals
   */
  secondaryNavbar: 150,

  // ============================================================================
  // OVERLAY LAYER (200-299)
  // Temporary full-screen or panel overlays: sheets, modals, dialogs
  // ============================================================================

  /**
   * Modal overlay (backdrop)
   * Used for: ModalContainer backdrop, FullScreenOverlay
   */
  modalOverlay: 200,

  /**
   * Modal content
   * Used for: ModalContainer content, AssetSearchModal
   */
  modalContent: 210,

  /**
   * Nested/stacked modals
   * Used for: ModalContainer (nested variant) - modals on top of other modals
   */
  modalNested: 220,

  /**
   * Sheet overlay (backdrop)
   * Used for: SheetOverlay - the semi-transparent backdrop behind sheets
   */
  sheetOverlay: 230,

  /**
   * Sheet content panel
   * Used for: SheetContent - the slide-out panel itself
   */
  sheetContent: 240,

  /**
   * Bottom select panel (mobile)
   * Used for: BottomSelectPanel - slide-up selection panel on mobile
   * Above sheet content but below popovers
   */
  bottomPanel: 250,

  // ============================================================================
  // POPOVER LAYER (300-399)
  // Floating UI elements: dropdowns, tooltips, popovers, context menus
  // These appear above everything else including sheets and modals
  // ============================================================================

  /**
   * Standard dropdowns, popovers, tooltips
   * Used for: SelectContent, DropdownMenuContent, PopoverContent, TooltipContent
   * Renders above all overlays including sheets and modals
   */
  popover: 300,

  /**
   * Nested popovers (e.g., submenu within a dropdown)
   * Used for: DropdownMenuSubContent, nested PopoverContent
   */
  popoverNested: 310,

  /**
   * Tooltips (highest priority)
   * Used for: TooltipContent, TooltipArrow
   * Ensures tooltips appear above all other UI elements
   */
  tooltip: 320,

  // ============================================================================
  // APP LAYER (900+)
  // Critical app-level overlays that must appear above everything
  // ============================================================================

  /**
   * App loader overlay
   * Used for: AppLoader - initial loading screen
   * Must cover all UI including popovers during app initialization
   */
  appLoader: 900,
} as const

/**
 * Type helper for z-index values
 */
export type ZIndexValue = (typeof ZIndex)[keyof typeof ZIndex]

/**
 * Returns a z-index value for stacked overlays.
 * Each overlay in the stack gets a higher z-index.
 * Base starts at modalOverlay (200), incrementing by 20.
 */
export function getOverlayZIndex(stackIndex: number) {
  return ZIndex.modalOverlay + stackIndex * 20
}

/**
 * IMPORTANT: How to Apply Z-Index Values
 *
 * Tailwind's JIT compiler cannot parse template literals with variables.
 * DO NOT use: className={`z-[${ZIndex.sheetContent}]`}
 *
 * Instead, use inline styles:
 * ✅ CORRECT: style={{ zIndex: ZIndex.sheetContent }}
 *
 * This ensures the z-index values are applied at runtime and work correctly.
 *
 * Example:
 * ```tsx
 * <div
 *   className="fixed inset-0 bg-black/50"
 *   style={{ zIndex: ZIndex.sheetOverlay }}
 * >
 * ```
 */
