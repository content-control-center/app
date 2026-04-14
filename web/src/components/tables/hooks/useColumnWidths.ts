import { useMemo, useState, useEffect, useRef } from 'react'
import type { ColumnConfig } from '../types'

type ColumnWidthResult = {
  cssVariables: Record<string, string>
  totalWidth: number
  hasOverflow: boolean
  stickyLeftPositions: Map<string, number>
  stickyRightPositions: Map<string, number>
}

/**
 * Calculate dynamic column widths based on container size
 *
 * Rules:
 * 1. If total fixed width < container width: auto columns expand to fill space
 * 2. If total fixed width > container width: all columns use minSize, enable scroll
 * 3. Multiple auto columns share remaining space equally
 */
export function useColumnWidths<TData extends Record<string, unknown>>(
  containerWidth: number,
  activeColumns: string[],
  columnConfigs: ColumnConfig<TData>[]
): ColumnWidthResult {
  return useMemo(() => {
    const cssVariables: Record<string, string> = {}
    const computedWidths = new Map<string, number>()

    // Helper function to calculate border width
    const getBorderWidth = (borderSide?: 'left' | 'right' | 'both'): number => {
      if (!borderSide) return 0
      return borderSide === 'both' ? 4 : 2
    }

    // Separate columns into fixed and auto
    const fixedColumns: Array<{ id: string; width: number; minWidth: number }> = []
    const autoColumns: Array<{ id: string; minWidth: number }> = []

    activeColumns.forEach((columnId) => {
      const config = columnConfigs.find((c) => c.id === columnId)
      if (!config) return

      // Add border width to configured sizes
      const borderWidth = getBorderWidth(config.borderSide)
      const minWidth = (config.minSize ?? 50) + borderWidth

      if (config.isAutoSize) {
        autoColumns.push({ id: columnId, minWidth })
      } else {
        const width = (config.size ?? 150) + borderWidth
        fixedColumns.push({ id: columnId, width, minWidth })
      }
    })

    // Calculate total fixed width
    const totalFixedWidth = fixedColumns.reduce((sum, col) => sum + col.width, 0)
    const totalMinWidth = fixedColumns.reduce((sum, col) => sum + col.minWidth, 0) +
                          autoColumns.reduce((sum, col) => sum + col.minWidth, 0)

    // Scenario 1: Overflow - total width exceeds container
    if (totalFixedWidth + autoColumns.length * 100 > containerWidth || totalMinWidth > containerWidth) {
      // Use minimum widths for all columns
      fixedColumns.forEach((col) => {
        const width = col.minWidth
        cssVariables[`--col-${col.id}-width`] = `${width}px`
        computedWidths.set(col.id, width)
      })
      autoColumns.forEach((col) => {
        const width = col.minWidth
        cssVariables[`--col-${col.id}-width`] = `${width}px`
        computedWidths.set(col.id, width)
      })

      const stickyLeftPositions = calculateStickyLeftPositions(activeColumns, columnConfigs, computedWidths)
      const stickyRightPositions = calculateStickyRightPositions(activeColumns, columnConfigs, computedWidths)

      return {
        cssVariables,
        totalWidth: totalMinWidth,
        hasOverflow: true,
        stickyLeftPositions,
        stickyRightPositions,
      }
    }

    // Scenario 2: No auto columns - use fixed widths
    if (autoColumns.length === 0) {
      fixedColumns.forEach((col) => {
        cssVariables[`--col-${col.id}-width`] = `${col.width}px`
        computedWidths.set(col.id, col.width)
      })

      const stickyLeftPositions = calculateStickyLeftPositions(activeColumns, columnConfigs, computedWidths)
      const stickyRightPositions = calculateStickyRightPositions(activeColumns, columnConfigs, computedWidths)

      // Calculate actual total width from all columns
      const actualTotalWidth = Array.from(computedWidths.values()).reduce((sum, w) => sum + w, 0)

      return {
        cssVariables,
        totalWidth: actualTotalWidth,
        hasOverflow: actualTotalWidth > containerWidth,
        stickyLeftPositions,
        stickyRightPositions,
      }
    }

    // Scenario 3: Underflow - distribute remaining space to auto columns
    const remainingSpace = containerWidth - totalFixedWidth
    const autoColumnWidth = Math.max(
      100, // Minimum width for auto columns
      Math.floor(remainingSpace / autoColumns.length)
    )

    // Set fixed column widths
    fixedColumns.forEach((col) => {
      cssVariables[`--col-${col.id}-width`] = `${col.width}px`
      computedWidths.set(col.id, col.width)
    })

    // Set auto column widths
    autoColumns.forEach((col, index) => {
      // Give last auto column any remaining pixels from floor rounding
      const isLast = index === autoColumns.length - 1
      const width = isLast
        ? remainingSpace - (autoColumnWidth * (autoColumns.length - 1))
        : autoColumnWidth

      const finalWidth = Math.max(col.minWidth, width)
      cssVariables[`--col-${col.id}-width`] = `${finalWidth}px`
      computedWidths.set(col.id, finalWidth)
    })

    const stickyLeftPositions = calculateStickyLeftPositions(activeColumns, columnConfigs, computedWidths)
    const stickyRightPositions = calculateStickyRightPositions(activeColumns, columnConfigs, computedWidths)

    // Calculate actual total width from all columns
    const actualTotalWidth = Array.from(computedWidths.values()).reduce((sum, w) => sum + w, 0)

    return {
      cssVariables,
      totalWidth: actualTotalWidth,
      hasOverflow: actualTotalWidth > containerWidth,
      stickyLeftPositions,
      stickyRightPositions,
    }
  }, [containerWidth, activeColumns, columnConfigs])
}

/**
 * Calculate sticky left positions based on computed widths
 * Iterates left to right, accumulating widths for left-sticky columns
 */
function calculateStickyLeftPositions<TData extends Record<string, unknown>>(
  activeColumns: string[],
  columnConfigs: ColumnConfig<TData>[],
  computedWidths: Map<string, number>
): Map<string, number> {
  const positions = new Map<string, number>()
  let cumulativeWidth = 0

  for (const columnId of activeColumns) {
    const config = columnConfigs.find((c) => c.id === columnId)
    if (!config) continue

    if (config.isSticky && config.stickyPosition === 'left') {
      positions.set(columnId, cumulativeWidth)
      cumulativeWidth += computedWidths.get(columnId) ?? 0
    }
  }

  return positions
}

/**
 * Calculate sticky right positions based on computed widths
 * Iterates right to left, accumulating widths for right-sticky columns
 */
function calculateStickyRightPositions<TData extends Record<string, unknown>>(
  activeColumns: string[],
  columnConfigs: ColumnConfig<TData>[],
  computedWidths: Map<string, number>
): Map<string, number> {
  const positions = new Map<string, number>()
  let cumulativeWidth = 0

  // Iterate from right to left
  for (let i = activeColumns.length - 1; i >= 0; i--) {
    const columnId = activeColumns[i]
    const config = columnConfigs.find((c) => c.id === columnId)
    if (!config) continue

    if (config.isSticky && config.stickyPosition === 'right') {
      positions.set(columnId, cumulativeWidth)
      cumulativeWidth += computedWidths.get(columnId) ?? 0
    }
  }

  return positions
}

/**
 * Hook to measure container width with ResizeObserver
 */
export function useContainerWidth(containerRef: React.RefObject<HTMLElement | null>): number {
  const [width, setWidth] = useState(0)
  const observerRef = useRef<ResizeObserver | null>(null)

  useEffect(() => {
    const element = containerRef.current
    if (!element) return

    // Initial measurement
    setWidth(element.clientWidth)

    // Set up ResizeObserver for dynamic updates
    observerRef.current = new ResizeObserver((entries) => {
      for (const entry of entries) {
        // Use borderBoxSize for more accurate measurements
        if (entry.borderBoxSize?.[0]) {
          setWidth(entry.borderBoxSize[0].inlineSize)
        } else {
          setWidth(entry.contentRect.width)
        }
      }
    })

    observerRef.current.observe(element)

    return () => {
      observerRef.current?.disconnect()
    }
  }, [containerRef])

  return width
}
