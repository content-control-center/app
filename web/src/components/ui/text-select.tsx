import * as React from 'react'
import * as SelectPrimitive from '@radix-ui/react-select'
import { cva } from 'class-variance-authority'
import { cn } from '@/lib'
import { ZIndex } from '@/config/zIndex.ts'
import { CaretDown, CaretUp } from '@phosphor-icons/react'

const popoverContentStyles =
  'bg-popover text-popover-foreground rounded-sm border-0 shadow-lg relative overflow-x-hidden overflow-y-auto'

const popoverAnimationStyles =
  'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2'

const selectContentStyles =
  'max-h-(--radix-select-content-available-height) min-w-[8rem] origin-(--radix-select-content-transform-origin)'

const selectViewportPopperStyles =
  'h-[var(--radix-select-trigger-height)] w-full min-w-[var(--radix-select-trigger-width)] scroll-my-1'

const selectItemStyles =
  "focus:bg-primary focus:text-primary-foreground [&_svg:not([class*='text-'])]:text-tertiary-foreground relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden select-none data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"

const selectScrollButtonStyles = 'flex cursor-default items-center justify-center py-1'

const selectTriggerVariants = cva(
  "data-[placeholder]:text-tertiary-foreground [&_svg:not([class*='text-'])]:text-tertiary-foreground aria-invalid:border-destructive flex w-fit items-center justify-between gap-2 font-medium whitespace-nowrap transition-[color,border-color,box-shadow] outline-none disabled:cursor-not-allowed disabled:opacity-50 *:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-2 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default:
          'bg-input border-b-1 border-border-tertiary focus-visible:border-foreground data-[state=open]:border-foreground rounded-none px-4 py-1 shadow-none text-[14px] leading-3 w-full transition-colors duration-200',
        primary:
          'bg-input-secondary border-b-2 border-quaternary focus-visible:border-foreground data-[state=open]:border-foreground rounded-none px-4 py-1 shadow-none text-[14px] leading-3 w-full transition-colors duration-200',
        ghost: 'border-0 rounded-md bg-transparent px-3 py-1 shadow-none text-sm',
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

type TextElement = {
  readonly id: string
  readonly displayValue: string
}

type CurrencySelectProps = {
  value: string
  placeholder?: string
  onValueChange: (value: string) => void
  elements: readonly TextElement[]
  disabled?: boolean
  className?: string
  id?: string
  variant?: 'default' | 'ghost' | 'primary' | null | undefined
  size?: 'default' | 'sm' | 'lg'
}

function TextSelect({
  value,
  placeholder,
  onValueChange,
  elements,
  disabled,
  className,
  id,
  variant = 'primary',
  size = 'lg',
}: CurrencySelectProps) {
  return (
    <SelectPrimitive.Root value={value} onValueChange={onValueChange} disabled={disabled}>
      <SelectPrimitive.Trigger
        id={id}
        className={cn(
          selectTriggerVariants({ variant, size }),
          'justify-between gap-3 pr-2 cursor-pointer',
          className
        )}
      >
        <SelectPrimitive.Value placeholder={placeholder ?? 'Select...'} />
        <div className="w-8 h-8 flex justify-center items-center text-center relative">
          <CaretDown className={'size-4'} />
        </div>
      </SelectPrimitive.Trigger>

      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          className={cn(popoverContentStyles, popoverAnimationStyles, selectContentStyles)}
          style={{ zIndex: ZIndex.popover }}
          position="popper"
        >
          <SelectPrimitive.ScrollUpButton className={selectScrollButtonStyles}>
            <CaretUp className="size-3" />
          </SelectPrimitive.ScrollUpButton>

          <SelectPrimitive.Viewport className={cn('p-1', selectViewportPopperStyles)}>
            {elements.map((el) => (
              <TextSelectItem
                key={el.id}
                value={el.id}
                isSelected={el.id === value}
                className="cursor-pointer justify-between gap-3 pr-2"
              >
                {el.displayValue}
              </TextSelectItem>
            ))}
          </SelectPrimitive.Viewport>

          <SelectPrimitive.ScrollDownButton className={selectScrollButtonStyles}>
            <CaretDown className="size-4" />
          </SelectPrimitive.ScrollDownButton>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  )
}

type TextSelectItemProps = React.ComponentProps<typeof SelectPrimitive.Item> & {
  isSelected: boolean
}

function TextSelectItem({ className, children, isSelected: _isSelected, ...props }: TextSelectItemProps) {
  return (
    <SelectPrimitive.Item className={cn(selectItemStyles, className)} {...props}>
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  )
}

export { TextSelect, selectTriggerVariants }
