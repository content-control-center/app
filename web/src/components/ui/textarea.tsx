import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib'

const textareaVariants = cva(
  'text-[14px] font-medium ' +
    'placeholder:text-tertiary-foreground selection:bg-selection/20 border-input flex w-full min-w-0 bg-transparent ' +
    'transition-[color,border-color,box-shadow] duration-300 outline-none resize-none overflow-hidden ' +
    'disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
  {
    variants: {
      variant: {
        primary:
          'bg-input-secondary rounded-none border-b-2 border-quaternary px-4 py-2 shadow-none focus-visible:border-foreground aria-invalid:border-destructive',
        default:
          'bg-input rounded-none border-b-1 border-quaternary px-4 py-2 shadow-none focus-visible:border-foreground aria-invalid:border-destructive',
      },
      rows: {
        sm: 'min-h-16',
        default: 'min-h-24',
        lg: 'min-h-40',
      },
    },
    defaultVariants: {
      variant: 'default',
      rows: 'default',
    },
  }
)

function resize(el: HTMLTextAreaElement | null) {
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}

const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<'textarea'> & VariantProps<typeof textareaVariants>
>(({ className, variant, rows, onChange, value, defaultValue, ...props }, ref) => {
  const innerRef = React.useRef<HTMLTextAreaElement | null>(null)

  React.useImperativeHandle(ref, () => innerRef.current as HTMLTextAreaElement, [])

  React.useLayoutEffect(() => {
    resize(innerRef.current)
  }, [value, defaultValue])

  return (
    <textarea
      ref={innerRef}
      data-slot="textarea"
      className={cn(textareaVariants({ variant, rows, className }))}
      value={value}
      defaultValue={defaultValue}
      onChange={(e) => {
        resize(e.currentTarget)
        onChange?.(e)
      }}
      {...props}
    />
  )
})

Textarea.displayName = 'Textarea'

// eslint-disable-next-line react-refresh/only-export-components
export { Textarea, textareaVariants }
