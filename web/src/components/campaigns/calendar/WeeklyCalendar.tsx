import { memo, useCallback, useMemo, useState } from 'react'
import type { Post } from '@/types/posts'
import { Button } from '@/components/ui/button'
import { CaretDownIcon, CaretLeftIcon, CaretRightIcon } from '@phosphor-icons/react'
import { useUpdatePost } from '@/hooks/usePosts'
import { postToPayload } from '@/services/api/posts'
import { PostCard } from './PostCard'
import { addDays, isSameDay, startOfWeek } from './date'
import { cn } from '@/lib'

type WeeklyCalendarProps = {
  campaignId: string
  posts: Post[]
  /** The anchor day from the route; the visible week is derived from it. */
  anchor: Date
  /** Navigate the calendar by changing the anchor day (updates the URL). */
  onAnchorChange: (anchor: Date) => void
}

const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
const UNSCHEDULED_KEY = 'unscheduled'
const DEFAULT_HOUR = 9

type Column = {
  key: string
  label: string
  /** Day-of-month, padded. */
  dateLabel: string
  day: Date
  isToday: boolean
  posts: Post[]
}

function WeeklyCalendarComponent({
  campaignId,
  posts,
  anchor,
  onAnchorChange,
}: WeeklyCalendarProps) {
  const [dragOverKey, setDragOverKey] = useState<string | null>(null)
  const [unscheduledOpen, setUnscheduledOpen] = useState(false)
  const today = useMemo(() => new Date(), [])
  const { mutate: updatePost } = useUpdatePost(campaignId)

  const weekStart = useMemo(() => startOfWeek(anchor), [anchor])
  const days = useMemo(
    () => Array.from({ length: 7 }, (_, i) => addDays(weekStart, i)),
    [weekStart],
  )

  const postsByDay = useMemo(() => {
    const map = new Map<string, Post[]>()
    for (const day of days) {
      map.set(day.toDateString(), [])
    }
    for (const post of posts) {
      if (!post.scheduled_at) continue
      const postDate = new Date(post.scheduled_at)
      for (const day of days) {
        if (isSameDay(postDate, day)) {
          map.get(day.toDateString())!.push(post)
          break
        }
      }
    }
    return map
  }, [posts, days])

  const unscheduledPosts = useMemo(
    () => posts.filter((p) => !p.scheduled_at),
    [posts],
  )

  const columns = useMemo<Column[]>(
    () =>
      days.map((day, i) => ({
        key: day.toDateString(),
        label: DAY_NAMES[i],
        dateLabel: day.getDate().toString().padStart(2, '0'),
        day,
        isToday: isSameDay(day, today),
        posts: postsByDay.get(day.toDateString()) ?? [],
      })),
    [days, today, postsByDay],
  )

  const handlePrev = useCallback(
    () => onAnchorChange(addDays(anchor, -7)),
    [anchor, onAnchorChange],
  )
  const handleNext = useCallback(
    () => onAnchorChange(addDays(anchor, 7)),
    [anchor, onAnchorChange],
  )
  const handleToday = useCallback(() => onAnchorChange(new Date()), [onAnchorChange])

  const applyDrop = useCallback(
    (post: Post, targetDay: Date | null) => {
      if (targetDay === null) {
        if (post.scheduled_at === null) return
        updatePost({
          id: post.id,
          payload: { ...postToPayload(post), scheduled_at: null },
        })
        return
      }
      const orig = post.scheduled_at ? new Date(post.scheduled_at) : null
      if (orig && isSameDay(orig, targetDay)) return
      const next = new Date(
        targetDay.getFullYear(),
        targetDay.getMonth(),
        targetDay.getDate(),
        orig ? orig.getHours() : DEFAULT_HOUR,
        orig ? orig.getMinutes() : 0,
        orig ? orig.getSeconds() : 0,
      )
      updatePost({
        id: post.id,
        payload: { ...postToPayload(post), scheduled_at: next.toISOString() },
      })
    },
    [updatePost],
  )

  const laneHandlers = useCallback(
    (key: string, targetDay: Date | null) => ({
      onDragOver: (e: React.DragEvent<HTMLDivElement>) => {
        e.preventDefault()
        e.dataTransfer.dropEffect = 'move'
        if (dragOverKey !== key) setDragOverKey(key)
      },
      onDragLeave: () => {
        setDragOverKey((k) => (k === key ? null : k))
      },
      onDrop: (e: React.DragEvent<HTMLDivElement>) => {
        e.preventDefault()
        const id = e.dataTransfer.getData('text/plain')
        setDragOverKey(null)
        if (!id) return
        const post = posts.find((p) => p.id === id)
        if (post) applyDrop(post, targetDay)
      },
    }),
    [dragOverKey, posts, applyDrop],
  )

  const weekEnd = addDays(weekStart, 6)
  const formatMonthRange = () => {
    const startMonth = weekStart.toLocaleDateString(undefined, { month: 'short', year: 'numeric' })
    const endMonth = weekEnd.toLocaleDateString(undefined, { month: 'short', year: 'numeric' })
    const startDay = weekStart.getDate()
    const endDay = weekEnd.getDate()
    if (startMonth === endMonth) {
      return `${startDay} – ${endDay} ${startMonth}`
    }
    return `${startDay} ${weekStart.toLocaleDateString(undefined, { month: 'short' })} – ${endDay} ${endMonth}`
  }

  return (
    <div className="flex flex-col h-full min-h-0 relative">
      {/* Navigator */}
      <div className="flex items-center justify-between shrink-0">
        <span className="text-[16px] font-medium">{formatMonthRange()}</span>
        <div className="flex items-center gap-3 px-0 py-2 shrink-0">
          <Button variant="ghost" size="sm" onClick={handleToday}>
            TODAY
          </Button>
          <Button variant="ghost" size="smIcon" onClick={handlePrev}>
            <CaretLeftIcon className="size-3.5" />
          </Button>
          <Button variant="ghost" size="smIcon" onClick={handleNext}>
            <CaretRightIcon className="size-3.5" />
          </Button>
        </div>
      </div>

      {/* Day columns — horizontally scrollable when cramped */}
      <div className="flex-1 min-h-0 overflow-x-auto">
        <div className="flex h-full">
          {columns.map((col, i) => (
            <div
              key={col.key}
              className={cn(
                'flex flex-col min-w-[200px] flex-1 min-h-0',
                i > 0 && 'border-l border-border',
              )}
            >
              {/* Column header — day name and date share one style:
                  semi-expanded (font-display) semi-bold. */}
              <div className="shrink-0 px-2 pt-2.5 pb-2 flex items-baseline gap-1.5">
                <span
                  className={cn(
                    'text-base font-display font-semibold leading-6 tabular-nums',
                    col.isToday && 'text-positive',
                  )}
                >
                  {col.label}
                </span>
                <span
                  className={cn(
                    'text-base font-display font-semibold leading-6 tabular-nums',
                    col.isToday && 'text-positive',
                  )}
                >
                  {col.dateLabel}
                </span>
              </div>

              {/* Posts lane — drop target */}
              <div
                {...laneHandlers(col.key, col.day)}
                className={cn(
                  'flex-1 min-h-0 overflow-y-auto px-2 pb-2 flex flex-col gap-2 items-stretch transition-colors',
                  dragOverKey === col.key && 'bg-secondary',
                )}
              >
                {col.posts.map((post) => (
                  <PostCard key={post.id} post={post} />
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Unscheduled posts — floating bottom panel, collapsed by default,
          overlapping the bottom of the calendar. Also a drop target that
          un-schedules any post dropped onto it. */}
      <div
        {...laneHandlers(UNSCHEDULED_KEY, null)}
        className={cn(
          'absolute inset-x-0 bottom-0 z-10 bg-background shadow-[0_-2px_12px_rgba(0,0,0,0.08)] transition-colors',
          dragOverKey === UNSCHEDULED_KEY && 'bg-secondary',
        )}
      >
        <button
          type="button"
          onClick={() => setUnscheduledOpen((o) => !o)}
          className="w-full flex items-center gap-2 px-3 py-2.5 text-left"
        >
          {unscheduledOpen ? (
            <CaretDownIcon className="size-4 text-tertiary-foreground" />
          ) : (
            <CaretRightIcon className="size-4 text-tertiary-foreground" />
          )}
          <span className="text-[16px] font-medium">Unscheduled</span>
          <span className="text-[16px] font-medium text-tertiary-foreground tabular-nums">
            {unscheduledPosts.length}
          </span>
        </button>

        {unscheduledOpen && (
          <div className="max-h-[45vh] min-h-[56px] overflow-y-auto px-3 pb-3 flex flex-wrap gap-2">
            {unscheduledPosts.length === 0 ? (
              <span className="self-center text-xs text-tertiary-foreground">
                No unscheduled posts
              </span>
            ) : (
              unscheduledPosts.map((post) => (
                <div key={post.id} className="w-[240px] shrink-0">
                  <PostCard post={post} />
                </div>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  )
}

export const WeeklyCalendar = memo(WeeklyCalendarComponent)
