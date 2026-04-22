import * as React from 'react'

import { cn } from '@/lib'
import { Icon } from '@/components/ui/icon'
import { ZIndex } from '@/config/zIndex'
import type { Tag } from '@/types/content'
import { useCreateTag, useTags } from '@/hooks/useTags'

type TagsInputProps = {
  value: string[]
  onChange: (ids: string[]) => void
  placeholder?: string
  disabled?: boolean
  id?: string
  className?: string
}

export function TagsInput({
  value,
  onChange,
  placeholder = 'Add a tag…',
  disabled,
  id,
  className,
}: TagsInputProps) {
  const { data: tags } = useTags()
  const { mutateAsync: createTag, isPending: isCreating } = useCreateTag()

  const [query, setQuery] = React.useState('')
  const [open, setOpen] = React.useState(false)
  const [highlight, setHighlight] = React.useState(0)
  const inputRef = React.useRef<HTMLInputElement | null>(null)

  const byId = React.useMemo(() => {
    const m = new Map<string, Tag>()
    for (const t of tags ?? []) m.set(t.id, t)
    return m
  }, [tags])

  const selected: Tag[] = value
    .map((id) => byId.get(id))
    .filter((t): t is Tag => !!t)

  const q = query.trim()
  const qLower = q.toLowerCase()

  const suggestions = React.useMemo(() => {
    const selectedSet = new Set(value)
    const all = (tags ?? []).filter((t) => !selectedSet.has(t.id))
    if (q === '') return all.slice(0, 8)
    return all
      .filter((t) => t.name.toLowerCase().includes(qLower))
      .slice(0, 8)
  }, [tags, value, q, qLower])

  const exactMatch = React.useMemo(
    () =>
      q === ''
        ? null
        : (tags ?? []).find((t) => t.name.toLowerCase() === qLower) ?? null,
    [tags, q, qLower]
  )

  const showCreate = q !== '' && !exactMatch
  const optionCount = suggestions.length + (showCreate ? 1 : 0)

  React.useEffect(() => {
    setHighlight(0)
  }, [query, open])

  const addId = (id: string) => {
    if (value.includes(id)) return
    onChange([...value, id])
  }

  const removeAt = (idx: number) => {
    const next = [...value]
    next.splice(idx, 1)
    onChange(next)
  }

  const handleCreate = async () => {
    if (q === '' || isCreating) return
    // If a selected tag already has this name, just clear the query.
    if (selected.some((t) => t.name.toLowerCase() === qLower)) {
      setQuery('')
      return
    }
    const created = await createTag({ name: q })
    addId(created.id)
    setQuery('')
  }

  const commitHighlight = () => {
    if (highlight < suggestions.length) {
      addId(suggestions[highlight].id)
      setQuery('')
      return
    }
    if (showCreate) {
      void handleCreate()
    }
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      if (exactMatch && !value.includes(exactMatch.id) && highlight >= suggestions.length) {
        addId(exactMatch.id)
        setQuery('')
        return
      }
      commitHighlight()
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setOpen(true)
      setHighlight((h) => Math.min(h + 1, Math.max(optionCount - 1, 0)))
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      setOpen(true)
      setHighlight((h) => Math.max(h - 1, 0))
      return
    }
    if (e.key === 'Escape') {
      setOpen(false)
      return
    }
    if (e.key === 'Backspace' && query === '' && value.length > 0) {
      e.preventDefault()
      removeAt(value.length - 1)
    }
  }

  return (
    <div className={cn('relative', className)}>
      <div
        className={cn(
          'flex flex-wrap items-center gap-1.5 min-h-10 bg-input rounded-none border-b-1 border-quaternary px-3 py-1.5 cursor-text',
          'focus-within:border-foreground transition-[border-color] duration-300',
          disabled && 'opacity-50 pointer-events-none'
        )}
        onClick={() => inputRef.current?.focus()}
      >
        {selected.map((t, i) => (
          <span
            key={t.id}
            className="inline-flex items-center gap-1.5 rounded-full bg-quaternary text-primary-foreground text-[12px] font-medium pl-2.5 pr-1 py-0.5"
          >
            {t.name}
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                removeAt(i)
              }}
              className="flex items-center justify-center size-4 rounded-full cursor-pointer hover:bg-foreground/10"
              aria-label={`Remove ${t.name}`}
              disabled={disabled}
            >
              <Icon name="x_mark" className="size-3" />
            </button>
          </span>
        ))}
        <input
          ref={inputRef}
          id={id}
          type="text"
          value={query}
          placeholder={selected.length === 0 ? placeholder : ''}
          onChange={(e) => {
            setQuery(e.target.value)
            setOpen(true)
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => {
            // delay so option mousedown can register
            window.setTimeout(() => setOpen(false), 120)
          }}
          onKeyDown={onKeyDown}
          disabled={disabled}
          className="flex-1 min-w-[8ch] bg-transparent outline-none text-[14px] font-medium placeholder:text-tertiary-foreground"
        />
      </div>

      {open && (optionCount > 0) && (
        <ul
          style={{ zIndex: ZIndex.popover }}
          className="absolute left-0 right-0 top-[calc(100%+2px)] bg-popover text-popover-foreground rounded-sm shadow-md py-1 max-h-64 overflow-y-auto"
        >
          {suggestions.map((t, i) => (
            <li key={t.id}>
              <button
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  addId(t.id)
                  setQuery('')
                  inputRef.current?.focus()
                }}
                onMouseEnter={() => setHighlight(i)}
                className={cn(
                  'w-full text-left text-sm px-2 py-1.5 rounded-sm cursor-pointer',
                  highlight === i ? 'bg-primary text-primary-foreground' : 'hover:bg-secondary'
                )}
              >
                {t.name}
              </button>
            </li>
          ))}
          {showCreate && (
            <li>
              <button
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  void handleCreate()
                  inputRef.current?.focus()
                }}
                onMouseEnter={() => setHighlight(suggestions.length)}
                disabled={isCreating}
                className={cn(
                  'w-full text-left text-sm px-2 py-1.5 rounded-sm cursor-pointer disabled:opacity-60',
                  highlight === suggestions.length
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-secondary'
                )}
              >
                Create “{q}”
              </button>
            </li>
          )}
        </ul>
      )}
    </div>
  )
}
