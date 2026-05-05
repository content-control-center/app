import type { ReactNode } from 'react'
import { cn } from '@/lib'
import { cva, type VariantProps } from 'class-variance-authority'

const pageContainerVariants = cva('w-full min-w-0', {
  variants: {
    variant: {
      default: 'h-full space-y-4 py-6',
      fullFlex: 'h-svh flex flex-col justify-stretch overflow-hidden',
      fullscreen: 'min-h-svh h-0 flex flex-col',
    },
  },
  defaultVariants: {
    variant: 'default',
  },
})

type PageContainerProps = {
  children: ReactNode
  className?: string
} & VariantProps<typeof pageContainerVariants>

export function PageContainer({ children, className, variant }: PageContainerProps) {
  return <div className={cn(pageContainerVariants({ variant }), className)}>{children}</div>
}
