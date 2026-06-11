// Date helpers for the campaign calendar.
//
// The calendar URL carries a single `anchor` date (YYYY-MM-DD). The visible
// range is *derived* from the anchor + the view granularity — e.g. the `week`
// view shows the Monday→Sunday week that contains the anchor. The anchor is a
// free day (it is NOT normalized to Monday), so switching granularity later
// only means swapping the slug while keeping the same anchor date.

export function startOfWeek(date: Date): Date {
  const d = new Date(date)
  const day = d.getDay()
  const diff = day === 0 ? 6 : day - 1 // Monday = start of week
  d.setDate(d.getDate() - diff)
  d.setHours(0, 0, 0, 0)
  return d
}

export function addDays(date: Date, days: number): Date {
  const d = new Date(date)
  d.setDate(d.getDate() + days)
  return d
}

export function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  )
}

// Parse a `YYYY-MM-DD` anchor into a *local* Date (midnight). Returns null for
// anything malformed or non-existent (e.g. 2026-02-31). We build the date from
// numeric parts rather than `new Date(string)` to avoid UTC-parsing day shifts.
export function parseAnchor(value: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!m) return null
  const year = Number(m[1])
  const month = Number(m[2])
  const day = Number(m[3])
  const d = new Date(year, month - 1, day)
  if (
    d.getFullYear() !== year ||
    d.getMonth() !== month - 1 ||
    d.getDate() !== day
  ) {
    return null
  }
  return d
}

export function formatAnchor(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
