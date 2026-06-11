'use client'

import * as React from 'react'
import * as SheetPrimitive from '@radix-ui/react-dialog'

import { cn } from '@/lib'
import { ZIndex } from '@/config/zIndex.ts'
import { X } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button.tsx'

function Sheet({ modal = true, ...props }: React.ComponentProps<typeof SheetPrimitive.Root>) {
  return <SheetPrimitive.Root data-slot="sheet" modal={modal} {...props} />
}

function SheetTrigger({ ...props }: React.ComponentProps<typeof SheetPrimitive.Trigger>) {
  return <SheetPrimitive.Trigger data-slot="sheet-trigger" {...props} />
}

function SheetClose({ ...props }: React.ComponentProps<typeof SheetPrimitive.Close>) {
  return <SheetPrimitive.Close data-slot="sheet-close" {...props} />
}

function SheetPortal({ ...props }: React.ComponentProps<typeof SheetPrimitive.Portal>) {
  return <SheetPrimitive.Portal data-slot="sheet-portal" {...props} />
}

const SheetOverlay = React.forwardRef<
  React.ComponentRef<typeof SheetPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof SheetPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <SheetPrimitive.Overlay
    ref={ref}
    data-slot="sheet-overlay"
    className={cn(
      'fixed inset-0 bg-background/70 backdrop-blur-xs transition-opacity duration-200',
      'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
      className)}
    style={{ zIndex: ZIndex.sheetOverlay }}
    {...props}
  />
))

function SheetContent({
  className,
  children,
  side = 'right',
  onOpenAutoFocus,
  ariaTitle,
  ...props
}: React.ComponentProps<typeof SheetPrimitive.Content> & {
  side?: 'top' | 'right' | 'bottom' | 'left' | 'fullscreen'
  ariaTitle?: string
}) {
  const contentRef = React.useRef<HTMLDivElement>(null)

  const handleOpenAutoFocus = React.useCallback(
    (e: Event) => {
      if (onOpenAutoFocus) {
        onOpenAutoFocus(e)
        if (e.defaultPrevented && contentRef.current) {
          contentRef.current.focus()
        }
      }
    },
    [onOpenAutoFocus]
  )

  return (
    <SheetPortal>
      <SheetOverlay />
      <SheetPrimitive.Content
        ref={contentRef}
        data-slot="sheet-content"
        aria-describedby={undefined}
        onOpenAutoFocus={handleOpenAutoFocus}
        tabIndex={-1}
        className={cn(
          'bg-popover data-[state=open]:animate-in data-[state=closed]:animate-out fixed flex flex-col transition ease-in-out data-[state=closed]:duration-200 data-[state=open]:duration-300 outline-none',
          side === 'fullscreen' &&
            'bg-background data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 inset-0 w-full h-full',
          side === 'right' &&
            'data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right inset-y-0 right-0 h-full w-full sm:w-120',
          side === 'left' &&
            'data-[state=closed]:slide-out-to-left data-[state=open]:slide-in-from-left inset-y-0 left-0 h-full w-full sm:w-120',
          side === 'top' &&
            'data-[state=closed]:slide-out-to-top data-[state=open]:slide-in-from-top inset-x-0 top-0 h-auto',
          side === 'bottom' &&
            'data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom inset-x-0 bottom-0 h-auto',
          className
        )}
        style={{ zIndex: ZIndex.sheetContent }}
        {...props}
      >
        <SheetPrimitive.Title className="sr-only">{ariaTitle ?? 'Sheet'}</SheetPrimitive.Title>
        {children}
        {side === 'fullscreen' ? (
          <SheetPrimitive.Close asChild>
            <Button
              variant="ghost"
              size="smIcon"
              className="absolute top-5 right-4 lg:top-10 lg:right-10"
            >
              <X className="size-6" />
              <span className="sr-only">Close</span>
            </Button>
          </SheetPrimitive.Close>
        ) : (
          <SheetPrimitive.Close asChild>
            <Button
              variant="ghost"
              size="smIcon"
              className="absolute top-3 right-3 lg:top-8 lg:right-8"
            >
              <X className="size-5" />
              <span className="sr-only">Close</span>
            </Button>
          </SheetPrimitive.Close>
        )}
      </SheetPrimitive.Content>
    </SheetPortal>
  )
}

function SheetHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="sheet-header"
      className={cn('flex flex-col gap-1.5 px-3 py-3 lg:px-8 lg:py-8 pb-3', className)}
      {...props}
    />
  )
}

function SheetFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="sheet-footer"
      className={cn('flex flex-col gap-2 px-3 py-3 lg:px-8 lg:py-8', className)}
      {...props}
    />
  )
}

function SheetTitle({ className, ...props }: React.ComponentProps<typeof SheetPrimitive.Title>) {
  return (
    <SheetPrimitive.Title
      data-slot="sheet-title"
      className={cn(
        'text-foreground font-display text-xl font-medium tracking-tight truncate leading-8 lg:text-2xl select-none',
        className
      )}
      {...props}
    />
  )
}

function SheetDescription({
  className,
  ...props
}: React.ComponentProps<typeof SheetPrimitive.Description>) {
  return (
    <SheetPrimitive.Description
      data-slot="sheet-description"
      className={cn('text-tertiary-foreground text-sm', className)}
      {...props}
    />
  )
}

export {
  Sheet,
  SheetPortal,
  SheetOverlay,
  SheetTrigger,
  SheetClose,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
}
