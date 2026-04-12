import { type ReactNode, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib'
import { Backdrop } from '@/components/ui/backdrop'
import { ZIndex } from '@/config/zIndex'
import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'

export interface ModalContainerProps {
  /**
   * Whether the modal is open/visible
   */
  isOpen: boolean

  /**
   * Callback when the modal should be closed
   */
  onClose: () => void

  /**
   * Whether the modal has padding inside or not
   */

  isContainer?: boolean

  /**
   * Modal title displayed in the header
   */
  title?: string

  /**
   * Modal content
   */
  children: ReactNode

  /**
   * Additional CSS classes for the modal content
   */
  className?: string

  /**
   * Whether to show the close button in header
   * @default true
   */
  showCloseButton?: boolean

  /**
   * Whether clicking the backdrop should close the modal
   * @default true
   */
  closeOnBackdropClick?: boolean

  /**
   * Whether pressing ESC should close the modal
   * @default true
   */
  closeOnEscape?: boolean

  /**
   * Modal size variant (width)
   * @default 'default'
   */
  size?: 'small' | 'default' | 'large' | 'xlarge' | 'full'

  /**
   * Modal height variant
   * @default 'auto'
   */
  height?: 'auto' | 'full' | 'large' | 'medium' | 'small'

  /**
   * Z-index level for the modal
   * Use 'default' (z-50) for regular modals
   * Use 'nested' (z-60) for modals that appear on top of the edit overlay
   * @default 'default'
   */
  zIndex?: 'default' | 'nested'

  /**
   * Explicit z-index numeric value that takes precedence over the `zIndex` preset.
   * Used by the overlay registry to assign computed z-index values.
   */
  zIndexOverride?: number
}

/**
 * Generic modal container component with:
 * - Smooth fade-in/fade-out transitions
 * - Background overlay with blur effect
 * - Header with title and close button
 * - Click outside to close
 * - ESC key to close
 * - Focus management and accessibility
 * - Multiple size variants
 * - Portal rendering for proper z-index handling
 */
export function ModalContainer({
  isOpen,
  onClose,
  title,
  children,
  className,
  showCloseButton = true,
  closeOnBackdropClick = true,
  closeOnEscape = true,
  isContainer = false,
  size = 'default',
  height = 'auto',
  zIndex = 'default',
  zIndexOverride,
}: ModalContainerProps) {
  const modalRef = useRef<HTMLDivElement>(null)
  const previousActiveElement = useRef<Element | null>(null)

  // Handle escape key
  useEffect(() => {
    if (!isOpen || !closeOnEscape) return

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose()
      }
    }

    document.addEventListener('keydown', handleEscape)
    return () => document.removeEventListener('keydown', handleEscape)
  }, [isOpen, closeOnEscape, onClose])

  // Handle focus management
  useEffect(() => {
    if (isOpen) {
      // Store the currently focused element
      previousActiveElement.current = document.activeElement

      // Focus the modal container
      if (modalRef.current) {
        modalRef.current.focus()
      }

      // Prevent body scroll
      document.body.style.overflow = 'hidden'
    } else {
      // Restore focus to the previously focused element
      if (previousActiveElement.current instanceof HTMLElement) {
        previousActiveElement.current.focus()
      }

      // Restore body scroll
      document.body.style.overflow = ''
    }

    return () => {
      // Cleanup on unmount
      document.body.style.overflow = ''
    }
  }, [isOpen])

  // Handle backdrop click
  const handleBackdropClick = (event: React.MouseEvent<HTMLDivElement>) => {
    if (closeOnBackdropClick && event.target === event.currentTarget) {
      onClose()
    }
  }

  // Size variants (width)
  const sizeClasses = {
    small: 'max-w-md',
    default: 'max-w-lg',
    large: 'max-w-2xl',
    xlarge: 'max-w-4xl',
    full: 'max-w-[95vw]',
  }

  // Height variants
  const heightClasses = {
    auto: '',
    small: 'max-h-[40vh]',
    medium: 'max-h-[60vh]',
    large: 'max-h-[80vh]',
    full: 'h-full',
  }

  // Backdrop alignment classes based on height
  const backdropAlignmentClasses = {
    auto: 'items-center justify-center',
    small: 'items-center justify-center',
    medium: 'items-center justify-center',
    large: 'items-center justify-center',
    full: 'items-stretch justify-center lg:py-20',
  }

  // Z-index values from centralized config
  const zIndexValues = {
    default: ZIndex.modalOverlay,
    nested: ZIndex.modalNested,
  }

  const resolvedZIndex = zIndexOverride ?? zIndexValues[zIndex]

  if (!isOpen) return null

  return createPortal(
    <Backdrop
      open={isOpen}
      onClick={handleBackdropClick}
      zIndex={resolvedZIndex}
      role="dialog"
      aria-modal={true}
      aria-labelledby={title ? 'modal-title' : undefined}
      className={cn('flex', 'animate-in fade-in-0 duration-300', backdropAlignmentClasses[height])}
    >
      <div
        ref={modalRef}
        className={cn(
          'relative w-full bg-popover rounded-none',
          'animate-in zoom-in-95 duration-300',
          'focus:outline-none',
          sizeClasses[size],
          heightClasses[height],
          className
        )}
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        {title && (
          <div className="flex flex-col gap-1.5 px-6 py-4 border-b">
            <h2 id="modal-title" className="font-semibold text-foreground leading-7">
              {title}
            </h2>
          </div>
        )}

        {/* Close button — positioned to match SheetContent */}
        {showCloseButton && (
          <Button
            variant="ghost"
            size="smIcon"
            onClick={onClose}
            className="absolute top-3 right-3 lg:top-8 lg:right-8"
            aria-label="Close modal"
          >
            <Icon name="x_mark" className="size-5 stroke-2" />
            <span className="sr-only">Close</span>
          </Button>
        )}

        {/* Content */}
        <div className={isContainer ? 'h-full' : 'p-6'}>{children}</div>
      </div>
    </Backdrop>,
    document.body
  )
}
