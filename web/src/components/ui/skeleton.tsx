import { cn } from '@/lib'
import type { ComponentProps } from 'react'

function Skeleton({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      data-slot="skeleton"
      className={cn('bg-quinary animate-pulse rounded-md', className)}
      {...props}
    />
  )
}

export { Skeleton }
