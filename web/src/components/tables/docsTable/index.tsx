import { memo, useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { Trash } from '@phosphor-icons/react'
import { VirtualTable } from '../VirtualTable'
import { TextCell } from '../TableCells'
import type { ColumnConfig } from '../types'
import type { Asset } from '@/types/content'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { statusToBadge } from '@/lib/assetStatus'
import { assetCategory, categoryLabel } from '@/lib/assetCategory'
import { formatTitle } from '@/lib'

type AssetRow = Asset & Record<string, unknown>

type AssetsTableProps = {
  assets: Asset[]
  onDelete: (id: string) => void
  emptyStateMessage?: string
  emptyStateActionLabel?: string
  onEmptyStateAction?: () => void
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function AssetsTableComponent({
  assets,
  onDelete,
  emptyStateMessage = 'No content assets',
  emptyStateActionLabel,
  onEmptyStateAction,
}: AssetsTableProps) {
  const data = assets as AssetRow[]

  const columnConfigs = useMemo<ColumnConfig<AssetRow>[]>(
    () => [
      {
        id: 'title',
        accessorKey: 'title',
        header: 'Title',
        isAutoSize: true,
        cell: (_value, row) => {
          const displayTitle = formatTitle(row.title)
          return (
            <Link
              to="/content-bank/$assetId"
              params={{ assetId: row.id }}
              className="block h-[34px] border-b-2 border-background px-3 leading-8 hover:underline"
            >
              <TextCell value={displayTitle} />
            </Link>
          )
        },
      },
      {
        id: 'status',
        accessorKey: 'status',
        header: 'Status',
        size: 130,
        minSize: 110,
        cell: (_value, row) => {
          const badge = statusToBadge(row.status)
          return (
            <Link
              to="/content-bank/$assetId"
              params={{ assetId: row.id }}
              className="block h-[34px] border-b-2 border-background px-3 leading-8"
            >
              <StatusBadge tone={badge.tone} label={badge.label} />
            </Link>
          )
        },
      },
      {
        id: 'type',
        accessorKey: 'type',
        header: 'Type',
        size: 120,
        minSize: 100,
        cell: (_value, row) => (
          <Link
            to="/content-bank/$assetId"
            params={{ assetId: row.id }}
            className="block h-[34px] border-b-2 border-background px-3 leading-8"
          >
            <TextCell value={categoryLabel(assetCategory(row))} />
          </Link>
        ),
      },
      {
        id: 'created_at',
        accessorKey: 'created_at',
        header: 'Created',
        size: 140,
        minSize: 120,
        cell: (_value, row) => (
          <Link
            to="/content-bank/$assetId"
            params={{ assetId: row.id }}
            className="block h-[34px] border-b-2 border-background px-3 leading-8"
          >
            <TextCell value={formatDate(row.created_at)} />
          </Link>
        ),
      },
      {
        id: 'updated_at',
        accessorKey: 'updated_at',
        header: 'Last Modified',
        size: 140,
        minSize: 120,
        cell: (_value, row) => (
          <Link
            to="/content-bank/$assetId"
            params={{ assetId: row.id }}
            className="block h-[34px] border-b-2 border-background px-3 leading-8"
          >
            <TextCell value={formatDate(row.updated_at)} />
          </Link>
        ),
      },
      {
        id: 'actions',
        header: '',
        size: 60,
        minSize: 60,
        sortable: false,
        cell: (_value, row) => (
          <div className="h-[34px] border-b-2 border-background px-3 flex items-center justify-center">
            <Button
              variant="ghost"
              size="xsIcon"
              onClick={(e) => {
                e.stopPropagation()
                onDelete(row.id)
              }}
            >
              <Trash className="size-4 text-tertiary-foreground hover:text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [onDelete],
  )

  const activeColumns = useMemo(
    () => ['title', 'status', 'type', 'created_at', 'updated_at', 'actions'],
    [],
  )

  return (
    <VirtualTable
      data={data}
      columnConfigs={columnConfigs}
      activeColumns={activeColumns}
      initialSorting={[{ id: 'title', desc: false }]}
      estimatedRowHeight={34}
      overscan={5}
      showFooter={false}
      fillHeight={false}
      emptyStateMessage={emptyStateMessage}
      emptyStateActionLabel={emptyStateActionLabel}
      onEmptyStateAction={onEmptyStateAction}
    />
  )
}

export const AssetsTable = memo(AssetsTableComponent)
