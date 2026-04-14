import type { ColumnConfig } from './types'

/**
 * Calculate totals based on column configuration
 */
export function calculateTotal<TData extends Record<string, unknown>>(
  rows: TData[],
  config: ColumnConfig<TData>
): unknown {
  if (!config.totals) return ''

  const { type, customCalculation } = config.totals

  if (type === 'custom' && customCalculation) {
    return customCalculation(rows)
  }

  const values = rows.map((row) => {
    const value = config.accessorKey
      ? (row as Record<string, unknown>)[config.accessorKey as string]
      : config.accessorFn?.(row)
    return typeof value === 'number' ? value : 0
  })

  switch (type) {
    case 'sum':
      return values.reduce((sum, val) => sum + val, 0)
    case 'average':
      return values.length > 0 ? values.reduce((sum, val) => sum + val, 0) / values.length : 0
    case 'count':
      return rows.length
    default:
      return ''
  }
}

/**
 * Calculate sticky left position for a column
 */
export function calculateStickyLeftPosition<TData extends Record<string, unknown>>(
  columnId: string,
  activeColumns: string[],
  columnConfigs: ColumnConfig<TData>[]
): number {
  let cumulativeWidth = 0

  for (const id of activeColumns) {
    if (id === columnId) break
    const config = columnConfigs.find((c) => c.id === id)
    if (config?.isSticky && config.stickyPosition === 'left') {
      cumulativeWidth += config.size || 150
    }
  }

  return cumulativeWidth
}

/**
 * Generate CSS variables for column widths using column IDs
 */
export function generateColumnWidthVars<TData extends Record<string, unknown>>(
  activeColumns: string[],
  columnConfigs: ColumnConfig<TData>[]
): Record<string, string> {
  const vars: Record<string, string> = {}

  activeColumns.forEach((columnId) => {
    const config = columnConfigs.find((c) => c.id === columnId)
    const width = config?.size || 150
    // Use column ID in CSS variable name
    vars[`--col-${columnId}-width`] = `${width}px`
  })

  return vars
}