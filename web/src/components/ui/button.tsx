import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib'
import { Spinner } from '@/components/ui/spinner'

const buttonVariants = cva(
  'relative inline-flex items-center cursor-pointer whitespace-nowrap shrink-0 outline-none ' +
    'gap-2 rounded-md text-[13px]/4 font-medium select-none transition-all ' +
    'disabled:pointer-events-none ' +
    "[&_svg:not([class*='stroke-'])]:stroke-[2] [&_svg:not([class*='size-'])]:size-3 [&_svg]:shrink-0 " +
    'focus-visible:inset-ring-[2px] focus-visible:inset-ring-ring aria-invalid:ring-destructive/20 aria-invalid:border-destructive ' +
    '[&_[data-spinner-container]]:rounded-md',
  {
    variants: {
      variant: {
        default:
          'bg-primary text-primary-foreground hover:bg-primary-foreground hover:text-primary ' +
          'data-[active=true]:bg-secondary data-[active=true]:inset-ring-[2px] data-[active=true]:inset-ring-ring ' +
          'data-[active=true]:hover:text-primary-foreground data-[active=true]:hover:bg-primary ' +
          '[&_[data-spinner-container]]:bg-primary [&_[data-spinner]]:bg-primary-foreground/20 [&_[data-spinner]:before]:bg-primary-foreground',
        defaultInverted:
          'bg-primary-foreground text-primary hover:bg-primary-foreground-elevated ' +
          'data-[active=true]:bg-primary-foreground-elevated ' +
          '[&_[data-spinner-container]]:bg-primary-foreground',
        tableHeader:
          'bg-transparent rounded-none text-tertiary-foreground font-regular text-xs hover:bg-table-row hover:text-primary-foreground ' +
          'data-[active=true]:text-primary-foreground',
        tableRow:
          'bg-transparent text-secondary-foreground hover:text-primary-foreground hover:bg-primary ' +
          'data-[active=true]:text-primary-foreground data-[active=true]:bg-primary',
        destructive:
          'bg-transparent text-destructive ' +
          'data-[active=true]:bg-destructive/10 ' +
          'data-[loading]:text-destructive/0 [&_[data-spinner]]:bg-destructive/20 [&_[data-spinner]:before]:bg-destructive ',
        destructiveInverted:
          'bg-destructive text-primary hover:bg-destructive/80 ' +
          'data-[active=true]:bg-destructive/80 ' +
          '[&_[data-spinner-container]]:bg-destructive',
        outline:
          'bg-transparent text-primary-foreground border hover:bg-quaternary hover:border-quaternary ' +
          'data-[active=true]:bg-quaternary data-[active=true]:border-quaternary ' +
          'data-[loading]:text-primary-foreground/0 [&_[data-spinner]]:bg-primary-foreground/20 [&_[data-spinner]:before]:bg-primary-foreground',
        secondary:
          'bg-primary text-primary-foreground hover:bg-secondary ' +
          'data-[active=true]:bg-secondary ' +
          '[&_[data-spinner-container]]:bg-primary [&_[data-spinner]]:bg-primary-foreground/20 [&_[data-spinner]:before]:bg-primary-foreground',
        ghost:
          'bg-transparent text-secondary-foreground hover:text-primary-foreground ' +
          'data-[active=true]:text-primary-foreground data-[active=true]:bg-quaternary ' +
          'data-[loading]:text-secondary-foreground/0 [&_[data-spinner]]:bg-secondary-foreground/20 [&_[data-spinner]:before]:bg-secondary-foreground',
        link: 'text-primary-foreground data-[active=true]:underline',

        container: '',
        searchBar:
          'bg-tertiary text-sm h-8 px-1.5 w-full truncate gap-3 justify-start text-tertiary-foreground hover:text-primary-foreground overflow-hidden ' +
          'data-[active=true]:text-primary-foreground data-[active=true]:bg-tertiary-elevated ' +
          '[&>div]:w-full [&>div]:flex [&>div]:items-center [&>div]:gap-3 ' +
          'lg:h-10 lg:px-2.5 lg:[&>div]:w-[232px]',
        menu:
          'bg-transparent h-8 px-1.5 w-full truncate gap-3 justify-start hover:bg-sidebar-secondary text-secondary-foreground text-sm hover:text-sidebar-primary-foreground overflow-hidden ' +
          'data-[active=true]:bg-sidebar-secondary data-[active=true]:text-sidebar-primary-foreground ' +
          '[&>div]:w-full [&>div]:flex [&>div]:items-center [&>div]:gap-2.5 [&_svg]:stroke-[1.5]' +
          'lg:h-10 lg:px-2.5 lg:[&>div]:w-[232px]',
      },

      size: {
        default: 'h-10 pt-[11px] pb-2 px-4',
        defaultIcon: 'h-10 p-0 w-10 justify-center',
        xsIcon: 'h-4 p-0 justify-center',
        sm: 'h-8 pt-[11px] pb-2 px-3',
        smIcon: 'h-8 p-0 w-8 justify-center',
        lg: 'h-10 pt-[13px] pb-3 px-4',
        lgIcon: 'h-10 p-0 w-10 justify-center',
        xl: 'h-11 pt-[17px] pb-4 px-6',
        xlIcon: 'h-11 p-0 w-11 justify-center',
        excluded: '',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  }
)

function Button({
  className,
  variant,
  size,
  asChild = false,
  loading = false,
  active = false,
  disabled,
  children,
  showEllipse = false,
  ...props
}: React.ComponentProps<'button'> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
    loading?: boolean
    active?: boolean
    showEllipse?: boolean
  }) {
  // When using asChild with loading, force it to render as a button
  const Comp = asChild && !loading ? Slot : 'button'

  // For asChild without loading, render as normal (single child for Slot)
  if (asChild && !loading) {
    return (
      <Comp
        data-slot="button"
        data-loading={loading ? 'true' : undefined}
        data-active={active ? 'true' : undefined}
        className={cn(buttonVariants({ variant, size, className }))}
        disabled={disabled}
        {...props}
      >
        {children}
      </Comp>
    )
  }

  // For normal button or loading state
  return (
    <Comp
      data-slot="button"
      data-loading={loading ? 'true' : undefined}
      data-active={active ? 'true' : undefined}
      className={cn(buttonVariants({ variant, size, className }))}
      disabled={disabled || loading}
      {...props}
    >
      <>
        {children}
        {showEllipse && (
          <div className="absolute top-3 right-3 pointer-events-none">
            <div className="w-2 h-2 rounded-full bg-positive border border-primary" />
          </div>
        )}
        {loading && (
          <span
            data-spinner-container
            className="absolute inset-0 flex items-center justify-center"
          >
            <Spinner />
          </span>
        )}
      </>
    </Comp>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export { Button, buttonVariants }
