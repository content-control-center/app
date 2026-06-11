import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib'
import { CalendarDots } from '@phosphor-icons/react'
import { Calendar } from '@/components/ui/calendar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const datePickerTriggerVariants = cva(
  'flex w-full items-center justify-between gap-2 bg-transparent font-medium text-[14px] ' +
    'transition-[color,border-color,box-shadow] duration-300 outline-none cursor-pointer ' +
    'disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
  {
    variants: {
      variant: {
        primary:
          'bg-input-secondary rounded-none border-b-2 border-quaternary px-4 shadow-none data-[state=open]:border-foreground focus-visible:border-foreground aria-invalid:border-destructive',
        default:
          'bg-input rounded-none border-b-1 border-quaternary px-4 shadow-none data-[state=open]:border-foreground focus-visible:border-foreground aria-invalid:border-destructive',
      },
      size: {
        sm: 'h-8',
        default: 'h-10',
        lg: 'h-12',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  }
)

type DatePickerProps = {
  value: string | null
  onChange: (next: string | null) => void
  placeholder?: string
  disabled?: boolean
  className?: string
  id?: string
} & VariantProps<typeof datePickerTriggerVariants>

function toDate(value: string | null): Date | undefined {
  if (!value) return undefined
  const d = new Date(value)
  return isNaN(d.getTime()) ? undefined : d
}

function toISODate(d: Date): string {
  // YYYY-MM-DD in local time
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function formatDisplay(d: Date): string {
  return d.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function DatePicker({
  value,
  onChange,
  placeholder = 'Pick a date',
  disabled,
  className,
  id,
  variant,
  size,
}: DatePickerProps) {
  const [open, setOpen] = React.useState(false)
  const selected = toDate(value)

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger
        id={id}
        disabled={disabled}
        className={cn(datePickerTriggerVariants({ variant, size }), className)}
      >
        <span
          className={cn(
            'truncate',
            selected ? 'text-foreground' : 'text-tertiary-foreground'
          )}
        >
          {selected ? formatDisplay(selected) : placeholder}
        </span>
        <CalendarDots className="size-4 text-tertiary-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="p-0">
        <Calendar
          mode="single"
          selected={selected}
          onSelect={(d) => {
            onChange(d ? toISODate(d) : null)
            setOpen(false)
          }}
        />
        {selected && (
          <button
            type="button"
            onClick={() => {
              onChange(null)
              setOpen(false)
            }}
            className="w-full border-t border-t-border-secondary px-3 py-2 text-xs font-medium text-tertiary-foreground hover:text-foreground cursor-pointer"
          >
            Clear
          </button>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export { DatePicker }
