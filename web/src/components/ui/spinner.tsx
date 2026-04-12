import { cn } from '@/lib'

export function Spinner({ className }: { className?: string }) {
  return (
    <div
      data-spinner
      className={cn(
        'relative w-6 h-[2px]',
        'bg-primary/20',
        "before:absolute before:content-['] before:inset-0 before:bg-primary before:animate-[spinner-line_0.7s_cubic-bezier(0,0,0.03,0.9)_infinite]",
        className
      )}
    />
  )
}
