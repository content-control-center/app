import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib'

const chipVariants = cva('inline-flex items-center text-[13px] leading-4 border font-medium px-4 py-[11px] rounded-lg', {
  variants: {
    variant: {
      default: 'bg-tertiary text-foreground border-transparent',
      muted: 'bg-transparent text-tertiary-foreground border border-senary-foreground',
    },
  },
  defaultVariants: {
    variant: 'default',
  },
})

type ChipProps = React.ComponentProps<'span'> & VariantProps<typeof chipVariants>

const Chip = React.forwardRef<HTMLSpanElement, ChipProps>(
  ({ className, variant, ...props }, ref) => (
    <span ref={ref} className={cn(chipVariants({ variant, className }))} {...props} />
  ),
)
Chip.displayName = 'Chip'

// eslint-disable-next-line react-refresh/only-export-components
export { Chip, chipVariants }
