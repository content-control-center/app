'use client'

import * as React from 'react'
import * as CheckboxPrimitive from '@radix-ui/react-checkbox'
import { CheckIcon } from '@phosphor-icons/react'

import { cn } from '@/lib'

export interface CheckboxProps extends React.ComponentPropsWithoutRef<
  typeof CheckboxPrimitive.Root
> {
  checkedClassName?: string
  iconClassName?: string
}

const Checkbox = React.forwardRef<React.ComponentRef<typeof CheckboxPrimitive.Root>, CheckboxProps>(
  ({ className, checkedClassName, iconClassName, ...props }, ref) => (
    <CheckboxPrimitive.Root
      ref={ref}
      className={cn(
        'peer bg-transparent h-4 w-4 shrink-0 rounded-none border-[1.5px] border-quaternary cursor-pointer',
        'data-[state=checked]:border-primary-foreground data-[state=checked]:text-primary-foreground',
        'data-[state=checked]:bg-foreground border-tertiary-foreground data-[state=checked]:border-foreground',
        checkedClassName,
        'focus-visible:outline-none focus-visible:border-2 focus-visible:border-primary-foreground',
        'disabled:cursor-default disabled:bg-ring/20',
        className
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        className={cn('flex items-center justify-center w-full h-full text-primary')}
      >
        <CheckIcon className={cn('size-4 text-current', iconClassName)} />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
)
Checkbox.displayName = CheckboxPrimitive.Root.displayName

export { Checkbox }
