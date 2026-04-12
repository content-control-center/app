import { useCallback, useEffect, useMemo, useRef, useState, memo, type JSX } from 'react'
import {
  type Column,
  type ColumnDef,
  type ColumnFiltersState,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  type Row,
  type SortingState,
  useReactTable,
} from '@tanstack/react-table'
import { useVirtualizer } from '@tanstack/react-virtual'
import * as ScrollAreaPrimitive from '@radix-ui/react-scroll-area'
import { cn } from '@/lib'
import type { VirtualTableProps, ColumnConfig } from './types'
import { calculateTotal } from './utils'
import { useColumnWidths, useContainerWidth } from './hooks/useColumnWidths'
import { Button } from '@/components/ui/button'
import { ScrollBar } from '@/components/ui/scroll-area'
import { ZIndex } from '@/config/zIndex.ts'
import { Icon } from '@/components/ui/icon'
import { FooterCell } from '@/components/tables/TableCells.tsx'
import { TableEmptyState } from './TableEmptyState'

function VirtualTableComponent<TData extends Record<string, unknown>>({
  data,
  columnConfigs,
  activeColumns,
  className,
  initialSorting = [],
  enableFiltering = true,
  enableGlobalFilter = true,
  estimatedRowHeight = 34,
  overscan = 5,
  showFooter = true,
  fillHeight = false,
  emptyStateMessage,
  emptyStateActionLabel,
  onEmptyStateAction,
}: VirtualTableProps<TData>) {
  const [sorting, setSorting] = useState<SortingState>(initialSorting)
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [globalFilter, setGlobalFilter] = useState('')
  const [spacerHeight, setSpacerHeight] = useState(0)
  const [contentFitsInSpace, setContentFitsInSpace] = useState(false)

  // Refs for measuring
  const containerRef = useRef<HTMLDivElement>(null)
  const viewportRef = useRef<HTMLDivElement>(null)
  const headerRef = useRef<HTMLDivElement>(null)
  const footerRef = useRef<HTMLDivElement>(null)

  // Measure container width for responsive column sizing
  const containerWidth = useContainerWidth(viewportRef)

  // Helper function to get column config by id
  const getColumnConfig = useCallback(
    (columnId: string) => {
      return columnConfigs.find((config) => config.id === columnId)
    },
    [columnConfigs]
  )

  // Create TanStack Table columns from configs
  const columns = useMemo<ColumnDef<TData>[]>(() => {
    const activeConfigs = activeColumns
      .map((columnId) => getColumnConfig(columnId))
      .filter(Boolean) as ColumnConfig<TData>[]

    return activeConfigs.map((config) => ({
      id: config.id,
      accessorKey: config.accessorKey,
      accessorFn: config.accessorFn,
      size: config.isAutoSize ? undefined : config.size,
      minSize: config.isAutoSize ? undefined : config.minSize,
      maxSize: config.isAutoSize ? undefined : config.maxSize,
      enableSorting: config.sortable !== false,
      sortUndefined: config.sortUndefined,
      header: ({ column }: { column: Column<TData, unknown> }) => {
        if (typeof config.header === 'function') {
          return config.header()
        }

        const sortDirection = column.getIsSorted()
        const alignment = config.alignment || 'left'

        if (config.sortable === false) {
          return (
            <div
              className={cn(
                '',
                alignment === 'right' ? 'text-right flex-row-reverse' : 'text-left'
              )}
            >
              {config.header}
            </div>
          )
        }

        return (
          <Button
            variant="tableHeader"
            size="sm"
            active={!!sortDirection}
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
            className={cn(
              'w-full justify-start gap-1 overflow-hidden',
              alignment === 'right' ? 'text-right flex-row-reverse' : 'text-left'
            )}
          >
            <span className="truncate">{config.header}</span>
            <div className="size-2.5 shrink-0 leading-0">
              {sortDirection === 'asc' && (
                <Icon name={'arrow_down_pointed'} className={'size-2.5 stroke-[2.5]'} />
              )}
              {sortDirection === 'desc' && (
                <Icon name={'arrow_up_pointed'} className={'size-2.5 stroke-[2.5]'} />
              )}
            </div>
          </Button>
        )
      },
      cell: ({ row }: { row: Row<TData> }) => {
        const value = config.accessorKey
          ? row.getValue(config.accessorKey as string)
          : row.getValue(config.id)
        const alignment = config.alignment || 'left'
        const cellClass = cn(
          alignment === 'right' ? 'text-right' : 'text-left',
          config.cellClassName ?? ''
        )

        if (config.cell) {
          return (
            <div
              className={cn(
                'h-[34px] border-b-2 pt-0.5 border-background leading-7 relative',
                cellClass
              )}
            >
              {config.cell(value, row.original)}
            </div>
          )
        }

        return <div className={cn('', cellClass)}>{String(value)}</div>
      },
      footer: config.totals
        ? ({ table }) => {
            const filteredRows = table.getFilteredRowModel().rows.map((row) => row.original)
            const result = calculateTotal(filteredRows, config)
            const alignment = config.alignment || 'left'
            const cellClass = alignment === 'right' ? 'text-right' : 'text-left'

            if (config.totals!.formatter && typeof result === 'number') {
              return (
                <div className={cn('h-8 px-3', cellClass)}>
                  <FooterCell className={'leading-8'} value={config.totals!.formatter(result)} />
                </div>
              )
            }

            return <div className={cn('px-3', cellClass)}>{String(result)}</div>
          }
        : undefined,
    }))
  }, [activeColumns, getColumnConfig])

  // Create TanStack Table instance
  const table = useReactTable({
    data,
    columns,
    onSortingChange: setSorting,
    onColumnFiltersChange: enableFiltering ? setColumnFilters : undefined,
    onGlobalFilterChange: enableGlobalFilter ? setGlobalFilter : undefined,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: enableFiltering ? getFilteredRowModel() : undefined,
    getSortedRowModel: getSortedRowModel(),
    state: {
      sorting,
      ...(enableFiltering && { columnFilters }),
      ...(enableGlobalFilter && { globalFilter }),
    },
  })

  const { rows } = table.getRowModel()

  // Row virtualizer
  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => viewportRef.current,
    estimateSize: () => estimatedRowHeight,
    overscan,
  })

  const virtualRows = rowVirtualizer.getVirtualItems()
  const totalSize = rowVirtualizer.getTotalSize()

  // Calculate dynamic column widths based on container size
  const {
    cssVariables: columnWidthVars,
    totalWidth,
    hasOverflow,
    stickyLeftPositions,
    stickyRightPositions,
  } = useColumnWidths(containerWidth, activeColumns, columnConfigs)

  // Get sticky left position for a column
  const getStickyLeftPosition = useCallback(
    (columnId: string) => stickyLeftPositions.get(columnId) ?? 0,
    [stickyLeftPositions]
  )

  // Get sticky right position for a column
  const getStickyRightPosition = useCallback(
    (columnId: string) => stickyRightPositions.get(columnId) ?? 0,
    [stickyRightPositions]
  )

  const hasData = rows.length > 0
  const hasFooter =
    showFooter &&
    table
      .getFooterGroups()
      .some((group) => group.headers.some((header) => header.column.columnDef.footer))

  // Calculate spacer height when fillHeight is enabled and check if content fits
  useEffect(() => {
    if (!containerRef.current || !viewportRef.current) return

    const updateMeasurements = () => {
      // Measure parent's available space (the grid cell)
      const parentHeight = containerRef.current?.parentElement?.clientHeight ?? 0
      const headerHeight = headerRef.current?.clientHeight ?? 0
      const footerHeight = hasFooter ? (footerRef.current?.clientHeight ?? 0) : 0
      const contentHeight = headerHeight + totalSize + footerHeight

      // Update spacer height for fillHeight mode
      if (fillHeight) {
        const viewportHeight = viewportRef.current?.clientHeight ?? 0
        const remainingHeight = viewportHeight - contentHeight
        setSpacerHeight(Math.max(0, remainingHeight))
      }

      // Check if content fits within parent's available space
      setContentFitsInSpace(contentHeight <= parentHeight)
    }

    updateMeasurements()

    // Observe parent element for resize, not ourselves
    const resizeObserver = new ResizeObserver(updateMeasurements)
    const parent = containerRef.current?.parentElement
    if (parent) {
      resizeObserver.observe(parent)
    }

    return () => resizeObserver.disconnect()
  }, [fillHeight, totalSize, hasFooter])

  return (
    <div
      ref={containerRef}
      className={cn(
        'flex flex-col w-full overflow-hidden bg-table-row box-border',
        // When no data, always fill height. When has data, use fillHeight prop or contentFitsInSpace logic
        !hasData || fillHeight
          ? 'flex-1 min-h-0'
          : contentFitsInSpace
            ? 'self-start'
            : 'flex-1 min-h-0',
        className
      )}
      style={columnWidthVars}
    >
      {/* Scrollable viewport */}
      <ScrollAreaPrimitive.Root className={cn('relative flex-1 min-h-0 overflow-hidden')}>
        <ScrollAreaPrimitive.Viewport
          ref={viewportRef}
          className={'h-full w-full rounded-[inherit]'}
        >
          {/* Header - sticky top */}
          <div
            ref={headerRef}
            className={cn(
              'sticky top-0 bg-table-header',
              `z-${ZIndex.stickyTableHeader}`,
              !hasData && 'pointer-events-none'
            )}
          >
            {table.getHeaderGroups().map((headerGroup) => (
              <div
                key={headerGroup.id}
                className="flex"
                style={{
                  width: hasOverflow ? `${totalWidth}px` : '100%',
                }}
              >
                {headerGroup.headers.map((header) => {
                  const columnConfig = getColumnConfig(header.column.id)
                  const isLeftSticky =
                    columnConfig?.isSticky && columnConfig?.stickyPosition === 'left'
                  const isRightSticky =
                    columnConfig?.isSticky && columnConfig?.stickyPosition === 'right'
                  const stickyLeftPosition = isLeftSticky
                    ? getStickyLeftPosition(header.column.id)
                    : 0
                  const stickyRightPosition = isRightSticky
                    ? getStickyRightPosition(header.column.id)
                    : 0

                  return (
                    <div
                      key={header.id}
                      className={cn(
                        'shrink-0 w-full grid border-b-2 border-background',
                        isLeftSticky && 'sticky bg-table-header',
                        isRightSticky && 'sticky bg-table-header',
                        columnConfig?.borderSide === 'left' && 'border-l-2',
                        columnConfig?.borderSide === 'right' && 'border-r-2',
                        columnConfig?.borderSide === 'both' && 'border-x-2'
                      )}
                      style={{
                        width: `var(--col-${header.column.id}-width)`,
                        ...(isLeftSticky && {
                          left: stickyLeftPosition,
                          zIndex: ZIndex.stickyTableColumnLeft,
                        }),
                        ...(isRightSticky && {
                          right: stickyRightPosition,
                          zIndex: ZIndex.stickyTableColumnRight,
                        }),
                      }}
                    >
                      {header.isPlaceholder
                        ? null
                        : flexRender(header.column.columnDef.header, header.getContext())}
                    </div>
                  )
                })}
              </div>
            ))}
          </div>

          {/* Empty state */}
          {!hasData && (
            <div className="flex flex-1 items-center justify-center w-full">
              <TableEmptyState
                message={emptyStateMessage}
                actionLabel={emptyStateActionLabel}
                onAction={onEmptyStateAction}
              />
            </div>
          )}

          {/* Body - virtualized rows */}
          {hasData && (
            <div
              className="relative"
              style={{
                height: `${totalSize}px`,
                width: hasOverflow ? `${totalWidth}px` : '100%',
              }}
            >
              {virtualRows.map((virtualRow) => {
                const row = rows[virtualRow.index]
                return (
                  <div
                    key={row.id}
                    className={cn(
                      'group flex absolute top-0 left-0 h-[34px] bg-table-row hover:bg-table-row-hover contain-strict'
                    )}
                    style={{
                      transform: `translateY(${virtualRow.start}px)`,
                      width: hasOverflow ? `${totalWidth}px` : '100%',
                    }}
                  >
                    {row.getVisibleCells().map((cell) => {
                      const columnConfig = getColumnConfig(cell.column.id)
                      const isLeftSticky =
                        columnConfig?.isSticky && columnConfig?.stickyPosition === 'left'
                      const isRightSticky =
                        columnConfig?.isSticky && columnConfig?.stickyPosition === 'right'
                      const stickyLeftPosition = isLeftSticky
                        ? getStickyLeftPosition(cell.column.id)
                        : 0
                      const stickyRightPosition = isRightSticky
                        ? getStickyRightPosition(cell.column.id)
                        : 0

                      return (
                        <div
                          key={cell.id}
                          className={cn(
                            'shrink-0 border-background contain-[layout_style_paint]',
                            isLeftSticky && 'sticky bg-table-row group-hover:bg-table-row-hover',
                            isRightSticky && 'sticky bg-table-row group-hover:bg-table-row-hover',
                            columnConfig?.borderSide === 'left' && 'border-l-2',
                            columnConfig?.borderSide === 'right' && 'border-r-2',
                            columnConfig?.borderSide === 'both' && 'border-x-2',
                            columnConfig?.cellClassName
                          )}
                          style={{
                            width: `var(--col-${cell.column.id}-width)`,
                            ...(isLeftSticky && {
                              left: stickyLeftPosition,
                              zIndex: ZIndex.stickyTableColumnLeft,
                            }),
                            ...(isRightSticky && {
                              right: stickyRightPosition,
                              zIndex: ZIndex.stickyTableColumnRight,
                            }),
                          }}
                        >
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </div>
                      )
                    })}
                  </div>
                )
              })}
            </div>
          )}

          {/* Spacer row to fill remaining height */}
          {hasData && fillHeight && spacerHeight > 0 && (
            <div
              className="flex bg-table-row"
              style={{
                height: `${spacerHeight}px`,
                width: hasOverflow ? `${totalWidth}px` : '100%',
              }}
            >
              {table.getHeaderGroups()[0]?.headers.map((header) => {
                const columnConfig = getColumnConfig(header.column.id)
                const isLeftSticky =
                  columnConfig?.isSticky && columnConfig?.stickyPosition === 'left'
                const isRightSticky =
                  columnConfig?.isSticky && columnConfig?.stickyPosition === 'right'
                const stickyLeftPosition = isLeftSticky
                  ? getStickyLeftPosition(header.column.id)
                  : 0
                const stickyRightPosition = isRightSticky
                  ? getStickyRightPosition(header.column.id)
                  : 0

                return (
                  <div
                    key={header.id}
                    className={cn(
                      'shrink-0 border-background',
                      isLeftSticky && 'sticky bg-table-row',
                      isRightSticky && 'sticky bg-table-row',
                      columnConfig?.borderSide === 'left' && 'border-l-2',
                      columnConfig?.borderSide === 'right' && 'border-r-2',
                      columnConfig?.borderSide === 'both' && 'border-x-2'
                    )}
                    style={{
                      width: `var(--col-${header.column.id}-width)`,
                      ...(isLeftSticky && {
                        left: stickyLeftPosition,
                        zIndex: ZIndex.stickyTableColumnLeft,
                      }),
                      ...(isRightSticky && {
                        right: stickyRightPosition,
                        zIndex: ZIndex.stickyTableColumnRight,
                      }),
                    }}
                  />
                )
              })}
            </div>
          )}

          {/* Footer - sticky bottom */}
          {hasData && hasFooter && (
            <div
              ref={footerRef}
              className={cn(
                'sticky bottom-0 bg-table-footer -mt-0.5',
                `z-${ZIndex.stickyTableFooter}`
              )}
            >
              {table.getFooterGroups().map((footerGroup) => (
                <div
                  key={footerGroup.id}
                  className="flex"
                  style={{
                    width: hasOverflow ? `${totalWidth}px` : '100%',
                  }}
                >
                  {footerGroup.headers.map((header) => {
                    const columnConfig = getColumnConfig(header.column.id)
                    const isLeftSticky =
                      columnConfig?.isSticky && columnConfig?.stickyPosition === 'left'
                    const isRightSticky =
                      columnConfig?.isSticky && columnConfig?.stickyPosition === 'right'
                    const stickyLeftPosition = isLeftSticky
                      ? getStickyLeftPosition(header.column.id)
                      : 0
                    const stickyRightPosition = isRightSticky
                      ? getStickyRightPosition(header.column.id)
                      : 0

                    return (
                      <div
                        key={header.id}
                        className={cn(
                          'shrink-0 border-t-2 border-background',
                          isLeftSticky && 'sticky bg-table-footer',
                          isRightSticky && 'sticky bg-table-footer',
                          columnConfig?.borderSide === 'left' && 'border-l-2',
                          columnConfig?.borderSide === 'right' && 'border-r-2',
                          columnConfig?.borderSide === 'both' && 'border-x-2'
                        )}
                        style={{
                          width: `var(--col-${header.column.id}-width)`,
                          ...(isLeftSticky && {
                            left: stickyLeftPosition,
                            zIndex: ZIndex.stickyTableColumnLeft,
                          }),
                          ...(isRightSticky && {
                            right: stickyRightPosition,
                            zIndex: ZIndex.stickyTableColumnRight,
                          }),
                        }}
                      >
                        {header.isPlaceholder
                          ? null
                          : flexRender(header.column.columnDef.footer, header.getContext())}
                      </div>
                    )
                  })}
                </div>
              ))}
            </div>
          )}
        </ScrollAreaPrimitive.Viewport>
        {hasData && <ScrollBar orientation="vertical" style={{ zIndex: ZIndex.scrollBar }} />}
        {hasData && hasOverflow && (
          <ScrollBar orientation="horizontal" style={{ zIndex: ZIndex.scrollBar }} />
        )}
        {hasData && <ScrollAreaPrimitive.Corner style={{ zIndex: ZIndex.scrollBar }} />}
      </ScrollAreaPrimitive.Root>
    </div>
  )
}

// Export memoized version
export const VirtualTable = memo(VirtualTableComponent) as <TData extends Record<string, unknown>>(
  props: VirtualTableProps<TData>
) => JSX.Element
