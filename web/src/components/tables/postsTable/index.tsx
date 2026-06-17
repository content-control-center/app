import { memo, useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { TrashIcon } from '@phosphor-icons/react'
import { VirtualTable } from '../VirtualTable'
import { TextCell } from '../TableCells'
import type { ColumnConfig } from '../types'
import type { Post } from '@/types/posts'
import { POST_STATUS_LABELS, DELETABLE_STATUSES } from '@/types/posts'
import { Button } from '@/components/ui/button'
import { formatTitle } from '@/lib'

type PostRow = Post & Record<string, unknown>

type PostsTableProps = {
  posts: Post[]
  campaignId: string
  onDelete: (id: string) => void
  emptyStateMessage?: string
  emptyStateActionLabel?: string
  onEmptyStateAction?: () => void
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return 'Not set'
  const date = new Date(dateStr)
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function formatRelativeDate(dateStr: string | null): string {
  if (!dateStr) return 'Not set'
  const now = new Date()
  const date = new Date(dateStr)
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startOfTarget = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  const diffDays = Math.round((startOfTarget.getTime() - startOfToday.getTime()) / (1000 * 60 * 60 * 24))

  if (diffDays === 0) return 'Today'
  if (diffDays === 1) return 'Tomorrow'
  if (diffDays === -1) return 'Yesterday'
  if (diffDays > 1) return `In ${diffDays} days`
  return `${Math.abs(diffDays)} days ago`
}

function PostsTableComponent({
  posts,
  campaignId,
  onDelete,
  emptyStateMessage = 'No posts',
  emptyStateActionLabel,
  onEmptyStateAction,
}: PostsTableProps) {
  const data = posts as PostRow[]

  const columnConfigs = useMemo<ColumnConfig<PostRow>[]>(
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
              to="/campaigns/$campaignId/posts/$postId"
              params={{ campaignId, postId: row.id }}
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
        size: 180,
        minSize: 140,
        cell: (_value, row) => (
          <div className="h-[34px] border-b-2 border-background px-3 leading-8">
            <TextCell value={POST_STATUS_LABELS[row.status] ?? row.status} />
          </div>
        ),
      },
      {
        id: 'platform',
        accessorFn: (row) => row.platform?.name ?? 'No platform',
        header: 'Platform',
        size: 150,
        minSize: 120,
        cell: (_value, row) =>
          row.platform ? (
            <div className="h-[34px] border-b-2 border-background px-3 leading-8">
              <TextCell value={row.platform.name} />
            </div>
          ) : (
            <div className="h-[34px] border-b-2 border-background px-3 leading-8">
              <span className="table-text text-tertiary-foreground">No platform</span>
            </div>
          ),
      },
      {
        id: 'scheduled_at',
        accessorKey: 'scheduled_at',
        header: 'Scheduled Date',
        size: 160,
        minSize: 130,
        cell: (_value, row) => (
          <div className="h-[34px] border-b-2 border-background px-3 leading-8">
            <TextCell value={formatDate(row.scheduled_at)} />
          </div>
        ),
      },
      {
        id: 'relative_time',
        accessorKey: 'scheduled_at',
        header: 'When',
        size: 140,
        minSize: 110,
        cell: (_value, row) => (
          <div className="h-[34px] border-b-2 border-background px-3 leading-8">
            <TextCell value={formatRelativeDate(row.scheduled_at)} />
          </div>
        ),
      },
      {
        id: 'actions',
        header: '',
        size: 60,
        minSize: 60,
        sortable: false,
        cell: (_value, row) => {
          const canDelete = DELETABLE_STATUSES.includes(row.status)
          if (!canDelete) return <div className="h-[34px] border-b-2 border-background" />
          return (
            <div className="h-[34px] border-b-2 border-background px-3 flex items-center justify-center">
              <Button
                variant="ghost"
                size="xsIcon"
                onClick={(e) => {
                  e.stopPropagation()
                  onDelete(row.id)
                }}
              >
                <TrashIcon className="size-4 text-tertiary-foreground hover:text-destructive" />
              </Button>
            </div>
          )
        },
      },
    ],
    [campaignId, onDelete],
  )

  const activeColumns = useMemo(
    () => ['title', 'status', 'platform', 'scheduled_at', 'relative_time', 'actions'],
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

export const PostsTable = memo(PostsTableComponent)