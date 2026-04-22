import { memo, useCallback, useMemo, useState } from 'react'
import type { Post } from '@/types/posts'
import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'
import { PostCard } from './PostCard'
import { cn } from '@/lib'

type WeeklyCalendarProps = {
  posts: Post[]
}

function startOfWeek(date: Date): Date {
  const d = new Date(date)
  const day = d.getDay()
  const diff = day === 0 ? 6 : day - 1 // Monday = start
  d.setDate(d.getDate() - diff)
  d.setHours(0, 0, 0, 0)
  return d
}

function addDays(date: Date, days: number): Date {
  const d = new Date(date)
  d.setDate(d.getDate() + days)
  return d
}

function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  )
}

const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

function WeeklyCalendarComponent({ posts }: WeeklyCalendarProps) {
  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date()))
  const today = useMemo(() => new Date(), [])

  const days = useMemo(() => {
    return Array.from({ length: 7 }, (_, i) => addDays(weekStart, i))
  }, [weekStart])

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

  const handlePrev = useCallback(() => setWeekStart((s) => addDays(s, -7)), [])
  const handleNext = useCallback(() => setWeekStart((s) => addDays(s, 7)), [])
  const handleToday = useCallback(() => setWeekStart(startOfWeek(new Date())), [])

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
    <div className="flex flex-col h-full min-h-0">
      {/* Navigator */}
      <div className="flex items-center justify-between">
        <span className="text-[16px] font-medium">{formatMonthRange()}</span>
        <div className="flex items-center gap-3 px-0 py-2 shrink-0">
          <Button variant="ghost" size="sm" onClick={handleToday}>
            TODAY
          </Button>
          <Button variant="ghost" size="smIcon" onClick={handlePrev}>
            <Icon name="chevron_left" className="size-3.5 stroke-[2.5]" />
          </Button>
          <Button variant="ghost" size="smIcon" onClick={handleNext}>
            <Icon name="chevron_right" className="size-3.5 stroke-[2.5]" />
          </Button>
        </div>
      </div>


      {/* Day rows */}
      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col">
        {days.map((day, i) => {
          const dayPosts = postsByDay.get(day.toDateString()) ?? []
          const isToday = isSameDay(day, today)

          return (
            <div
              key={day.toISOString()}
              className={cn(
                'flex border-t border-border min-h-[72px]',
                i === 6 && 'border-b',
              )}
            >
              {/* Day label */}
              <div
                className={'w-10 shrink-0 py-2.5 pr-3 flex flex-col items-start'}
              >
                <span className={cn("text-xs text-tertiary-foreground", isToday && 'text-positive')}>
                  {DAY_NAMES[i]}
                </span>
                <span
                  className={cn(
                    'text-lg font-display font-medium leading-6',
                    isToday && 'text-positive',
                  )}
                >
                  {day.getDate()}
                </span>
              </div>

              {/* Posts lane */}
              <div className="flex-1 min-w-0 py-2 flex flex-wrap gap-2 items-start">
                {dayPosts.map((post) => (
                  <PostCard key={post.id} post={post} />
                ))}
              </div>
            </div>
          )
        })}

        {/* Unscheduled posts */}
        {unscheduledPosts.length > 0 && (
          <div className="flex border-t border-border min-h-[72px]">
            <div className="w-10 shrink-0 py-2.5 pr-3 flex flex-col items-start">
              <span className="text-xs text-tertiary-foreground">No date</span>
            </div>
            <div className="flex-1 min-w-0 py-2 flex flex-wrap gap-2 items-start">
              {unscheduledPosts.map((post) => (
                <PostCard key={post.id} post={post} />
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export const WeeklyCalendar = memo(WeeklyCalendarComponent)
