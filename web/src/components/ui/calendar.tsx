import * as React from 'react'
import { Icon } from '@/components/ui/icon'
import { DayButton, DayPicker, getDefaultClassNames } from 'react-day-picker'

import { cn } from '@/lib'
import { Button, buttonVariants } from '@/components/ui/button'

function Calendar({
  className,
  classNames,
  showOutsideDays = true,
  captionLayout = 'label',
  buttonVariant = 'ghost',
  formatters,
  components,
  ...props
}: React.ComponentProps<typeof DayPicker> & {
  buttonVariant?: React.ComponentProps<typeof Button>['variant']
}) {
  const defaultClassNames = getDefaultClassNames()

  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      footer={
        <Button
          variant="outline"
          className="border-x-0 border-b-0 w-full border-t-border-secondary"
          onClick={(e) => {
            if (props.mode === 'single' && props.onSelect) {
              const today = new Date()
              props.onSelect(today, today, {}, e)
            }
          }}
        >
          TODAY
        </Button>
      }
      className={cn(
        'bg-popover border-0 group/calendar [--cell-size:--spacing(8)] in-data-[slot=card-content]:bg-transparent in-data-[slot=popover-content]:bg-transparent',
        String.raw`rtl:**:[.rdp-button\_next>svg]:rotate-180`,
        String.raw`rtl:**:[.rdp-button\_previous>svg]:rotate-180`,
        className
      )}
      captionLayout={captionLayout}
      formatters={{
        formatMonthDropdown: (date) => date.toLocaleString('default', { month: 'short' }),
        ...formatters,
      }}
      classNames={{
        root: cn('w-fit', defaultClassNames.root),
        months: cn('flex gap-4 flex-col md:flex-row relative px-3 pb-3', defaultClassNames.months),
        month: cn('flex flex-col w-full gap-4', defaultClassNames.month),
        nav: cn(
          'flex items-center gap-1 w-full absolute top-0 inset-x-0 justify-between h-10 border-b-quaternary border-b-1 pointer-events-none',
          defaultClassNames.nav
        ),
        button_previous: cn(
          buttonVariants({ variant: buttonVariant }),
          'size-(--cell-size) aria-disabled:opacity-50 p-0 select-none pointer-events-auto',
          defaultClassNames.button_previous
        ),
        button_next: cn(
          buttonVariants({ variant: buttonVariant }),
          'size-(--cell-size) aria-disabled:opacity-50 p-0 select-none pointer-events-auto',
          defaultClassNames.button_next
        ),
        month_caption: cn(
          'flex items-center justify-center h-10 w-full px-(--cell-size)',
          defaultClassNames.month_caption
        ),
        dropdowns: cn(
          'w-full flex items-center text-sm font-medium justify-center h-(--cell-size) gap-1.5',
          defaultClassNames.dropdowns
        ),
        dropdown_root: cn(
          'relative has-focus:border-ring border-0 shadow-lg has-focus:ring-ring/50 has-focus:ring-[3px] rounded-none',
          defaultClassNames.dropdown_root
        ),
        dropdown: cn('absolute bg-popover inset-0 opacity-0', defaultClassNames.dropdown),
        caption_label: cn(
          'select-none font-medium',
          captionLayout === 'label'
            ? 'text-sm'
            : 'rounded-md pl-2 pr-1 flex items-center gap-1 text-sm h-8 [&>svg]:text-tertiary-foreground [&>svg]:size-3.5',
          defaultClassNames.caption_label
        ),
        table: 'w-full border-collapse',
        weekdays: cn('flex', defaultClassNames.weekdays),
        weekday: cn(
          'text-tertiary-foreground rounded-md flex-1 font-normal text-[0.8rem] select-none',
          defaultClassNames.weekday
        ),
        week: cn('flex w-full mt-2', defaultClassNames.week),
        week_number_header: cn('select-none w-(--cell-size)', defaultClassNames.week_number_header),
        week_number: cn(
          'text-[0.8rem] select-none text-tertiary-foreground',
          defaultClassNames.week_number
        ),
        day: cn(
          'relative w-full h-full p-0 text-center [&:first-child[data-selected=true]_button]:rounded-l-md [&:last-child[data-selected=true]_button]:rounded-r-md group/day aspect-square select-none',
          defaultClassNames.day
        ),
        range_start: cn('rounded-l-md bg-primary', defaultClassNames.range_start),
        range_middle: cn('rounded-none bg-secondary', defaultClassNames.range_middle),
        range_end: cn('rounded-r-md bg-primary', defaultClassNames.range_end),
        today: cn('rounded-md data-[selected=true]:rounded-none', defaultClassNames.today),
        outside: cn(
          'text-tertiary-foreground aria-selected:text-tertiary-foreground',
          defaultClassNames.outside
        ),
        disabled: cn('text-tertiary-foreground', defaultClassNames.disabled),
        hidden: cn('invisible', defaultClassNames.hidden),
        ...classNames,
      }}
      components={{
        Root: ({ className, rootRef, ...props }) => {
          return <div data-slot="calendar" ref={rootRef} className={cn(className)} {...props} />
        },
        Chevron: ({ className, orientation }) => {
          const name =
            orientation === 'left'
              ? 'chevron_left'
              : orientation === 'right'
                ? 'chevron_right'
                : 'chevron_down'
          return <Icon name={name} className={cn('size-4', className)} />
        },
        DayButton: CalendarDayButton,
        WeekNumber: ({ children, ...props }) => {
          return (
            <td {...props}>
              <div className="flex size-(--cell-size) items-center justify-center text-center">
                {children}
              </div>
            </td>
          )
        },
        ...components,
      }}
      {...props}
    />
  )
}

function CalendarDayButton({
  className,
  day,
  modifiers,
  ...props
}: React.ComponentProps<typeof DayButton>) {
  const defaultClassNames = getDefaultClassNames()

  const ref = React.useRef<HTMLButtonElement>(null)
  React.useEffect(() => {
    if (modifiers.focused) ref.current?.focus()
  }, [modifiers.focused])

  return (
    <Button
      ref={ref}
      variant="ghost"
      size="lgIcon"
      data-day={day.date.toLocaleDateString()}
      data-selected-single={
        modifiers.selected &&
        !modifiers.range_start &&
        !modifiers.range_end &&
        !modifiers.range_middle
      }
      data-range-start={modifiers.range_start}
      data-range-end={modifiers.range_end}
      data-range-middle={modifiers.range_middle}
      data-outside={modifiers.outside}
      data-disabled={modifiers.disabled}
      data-today={modifiers.today}
      className={cn(
        'flex aspect-square size-auto w-full min-w-(--cell-size) flex-col gap-1',
        'leading-none font-normal font-mono border-0',
        'focus:ring-0 focus:outline-none focus-visible:ring-0 focus-visible:ring-offset-0 focus-visible:outline-none focus-visible:border-0',
        'dark:hover:text-primary-foreground',
        'data-[selected-single=true]:bg-foreground data-[selected-single=true]:text-background data-[selected-single=true]:rounded-none',
        'data-[range-start=true]:rounded-md data-[range-start=true]:rounded-l-md data-[range-start=true]:bg-foreground data-[range-start=true]:text-background',
        'data-[range-middle=true]:rounded-none data-[range-middle=true]:bg-secondary data-[range-middle=true]:text-secondary-foreground',
        'data-[range-end=true]:rounded-md data-[range-end=true]:rounded-r-md data-[range-end=true]:bg-foreground data-[range-end=true]:text-background',
        'group-data-[focused=true]/day:border-ring group-data-[focused=true]/day:ring-ring/50 group-data-[focused=true]/day:relative group-data-[focused=true]/day:z-10 group-data-[focused=true]/day:ring-[3px]',
        "data-[today=true]:font-bold data-[today=true]:after:content-[''] data-[today=true]:after:absolute data-[today=true]:after:bottom-1.5 data-[today=true]:after:left-1/2 data-[today=true]:after:-translate-x-1/2 data-[today=true]:after:w-[3px] data-[today=true]:after:h-[3px] data-[today=true]:after:bg-foreground",
        'data-[outside=true]:text-tertiary-foreground data-[disabled=true]:text-tertiary-foreground [&[data-selected-single=true][data-today=true]]:after:bg-background [&>span]:text-xs',
        defaultClassNames.day,
        className
      )}
      {...props}
    />
  )
}

export { Calendar, CalendarDayButton }
