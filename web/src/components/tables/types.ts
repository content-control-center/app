import type { ReactNode } from 'react'
import type { SortingState } from '@tanstack/react-table'

// Totals configuration for columns
export type TotalsConfig<T> = {
  type: 'sum' | 'average' | 'count' | 'custom'
  formatter?: (value: number) => string
  customCalculation?: (rows: T[]) => unknown
  showInTotalsRow?: boolean
}

// Base column configuration interface
export type ColumnConfig<T> = {
  id: string
  accessorKey?: keyof T | string
  accessorFn?: (row: T) => unknown
  header: string | (() => ReactNode)
  size?: number
  maxSize?: number
  minSize?: number
  isAutoSize?: boolean
  alignment?: 'left' | 'right'
  borderSide?: 'left' | 'right' | 'both'
  cell?: (value: unknown, row: T) => ReactNode
  sortable?: boolean
  sortUndefined?: 'first' | 'last' | false
  isSticky?: boolean
  stickyPosition?: 'left' | 'right' // NEW: specify which side for sticky
  totals?: TotalsConfig<T>
  cellClassName?: string
}

// Virtual table specific props
export type VirtualTableProps<TData extends Record<string, unknown>> = {
  data: TData[]
  columnConfigs: ColumnConfig<TData>[]
  activeColumns: string[]
  className?: string
  initialSorting?: SortingState
  enableFiltering?: boolean
  enableGlobalFilter?: boolean
  estimatedRowHeight?: number
  overscan?: number
  showFooter?: boolean
  fillHeight?: boolean
  emptyStateMessage?: string
  emptyStateActionLabel?: string
  onEmptyStateAction?: () => void
}
