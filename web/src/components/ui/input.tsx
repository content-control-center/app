import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib'

const inputVariants = cva(
  'text-[14px] font-medium ' +
    'placeholder:text-tertiary-foreground selection:bg-selection/20 border-input flex w-full min-w-0 bg-transparent ' +
    'transition-[color,border-color,box-shadow] duration-300 outline-none file:inline-flex ' +
    'file:text-foreground file:border-0 file:bg-transparent file:font-medium ' +
    'disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50' +
    '[appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none',
  {
    variants: {
      variant: {
        primary:
          'bg-input-secondary rounded-none border-b-2 border-quaternary px-4 py-1 leading-3 shadow-none focus-visible:border-foreground aria-invalid:border-destructive',
        default:
          'bg-input rounded-none border-b-1 border-quaternary px-4 py-1 shadow-none focus-visible:border-foreground aria-invalid:border-destructive',
        cellInline:
          'table-text text-right border-0 bg-transparent p-0 outline-none focus:outline-none focus-visible:ring-0 focus-visible:border-0 shadow-none text-[oklch(0.4895_0.2063_260.59)]',
        search:
          'bg-transparent rounded-none border-0 px-4 py-1 leading-3 shadow-none aria-invalid:border-destructive',
      },
      inputSize: {
        sm: 'h-8',
        default: 'h-10',
        lg: 'h-12',
      },
    },
    defaultVariants: {
      variant: 'default',
      inputSize: 'default',
    },
  }
)

const Input = React.forwardRef<
  HTMLInputElement,
  React.ComponentProps<'input'> & VariantProps<typeof inputVariants>
>(({ className, variant, inputSize, type, ...props }, ref) => {
  return (
    <input
      ref={ref}
      type={type}
      data-slot="input"
      className={cn(inputVariants({ variant, inputSize, className }))}
      {...props}
    />
  )
})

Input.displayName = 'Input'

// eslint-disable-next-line react-refresh/only-export-components
export { Input, inputVariants }
