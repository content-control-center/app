import * as React from 'react'
import { cn } from '@/lib'

type BackdropProps = {
  /**
   * Whether the backdrop is visible
   */
  open: boolean
  /**
   * Callback when the backdrop is clicked
   */
  onClick?: (event: React.MouseEvent<HTMLDivElement>) => void
  /**
   * z-index value for the backdrop
   */
  zIndex?: number
  /**
   * Additional CSS classes to apply
   */
  className?: string
  /**
   * Custom inline styles
   */
  style?: React.CSSProperties
  /**
   * ARIA role for accessibility (e.g., 'dialog')
   */
  role?: string
  /**
   * Whether this is a modal (ARIA attribute)
   */
  'aria-modal'?: boolean
  /**
   * ID of the element that labels this backdrop (ARIA attribute)
   */
  'aria-labelledby'?: string
  /**
   * Children to render inside the backdrop
   */
  children?: React.ReactNode
}

/**
 * Backdrop overlay component
 * Renders a full-screen semi-transparent backdrop with blur effect
 * Can be used for modals, sheets, drawers, and other overlay UI
 * Supports ARIA attributes for accessibility
 *
 * Note: For Radix-based components (Sheet, Dialog), prefer using their native
 * Overlay primitives with backdropStyles. Use this component for custom overlays.
 */
export function Backdrop({
  open,
  onClick,
  zIndex,
  className,
  style,
  role,
  'aria-modal': ariaModal,
  'aria-labelledby': ariaLabelledby,
  children,
}: BackdropProps) {
  return (
    <div
      data-state={open ? 'open' : 'collapsed'}
      className={cn(
        'fixed inset-0 bg-background/70 backdrop-blur-xs transition-opacity duration-200',
        'data-[state=open]:opacity-100',
        'data-[state=collapsed]:opacity-0 data-[state=collapsed]:pointer-events-none',
        className
      )}
      style={{
        zIndex,
        ...style,
      }}
      onClick={onClick}
      role={role}
      aria-modal={ariaModal}
      aria-labelledby={ariaLabelledby}
      aria-hidden={role ? undefined : 'true'}
    >
      {children}
    </div>
  )
}
