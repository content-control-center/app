import { memo } from 'react'
import { cn } from '@/lib'

// Text display cell
type TextCellProps = {
  value: string | null
  className?: string
}

export const TextCell = memo(function TextCell({ value, className }: TextCellProps) {
  const isFilled = value !== null && value !== ''

  return (
    <span className={cn('table-text', !isFilled && 'text-tertiary-foreground', className)}>
      {isFilled ? value : '—'}
    </span>
  )
})

// Loading display cell
type LoadingCellProps = {
  className?: string
}

export const LoadingCell = memo(function LoadingCell({ className }: LoadingCellProps) {
  return <span className={cn('table-text', 'text-tertiary-foreground', className)}>Loading...</span>
})

// Numeric display cell
type NumericCellProps = {
  value: number | null
  rounding: number
  className?: string
  isStale?: boolean
}

export const NumericCell = memo(function NumericCell({
  value,
  rounding,
  className,
  isStale,
}: NumericCellProps) {
  const isFilled = value !== null

  return (
    <span
      className={cn(
        'table-text font-mono tracking-[0.05em]',
        !isFilled && 'text-tertiary-foreground',
        isStale && 'opacity-60',
        className
      )}
    >
      {isFilled
        ? value.toLocaleString(undefined, {
            minimumFractionDigits: rounding,
            maximumFractionDigits: rounding,
          })
        : '—'}
    </span>
  )
})

// Numeric display cell
type NumericChangeCellProps = {
  value: number | null
  rounding: number
  className?: string
  isStale?: boolean
  opinion?: 'judgement' | 'neutral'
}

export const NumericChangeCell = memo(function NumericCell({
  value,
  rounding,
  className,
  isStale,
  opinion = 'neutral',
}: NumericChangeCellProps) {
  const isFilled = value !== null
  const symbol = value !== null && value > 0 ? '+' : ''

  let fontColoring
  switch (opinion) {
    case 'judgement':
      fontColoring = !isFilled || value === 0 ? '' : value > 0 ? 'text-positive' : 'text-negative'
      break
    default:
      fontColoring = ''
  }

  return (
    <span
      className={cn(
        'table-text font-mono tracking-[0.05em]',
        !isFilled && 'text-tertiary-foreground',
        isFilled && fontColoring,
        isStale && 'opacity-60',
        className
      )}
    >
      {isFilled
        ? `${symbol}${value.toLocaleString(undefined, {
            minimumFractionDigits: rounding,
            maximumFractionDigits: rounding,
          })}`
        : '—'}
    </span>
  )
})

// Percent display cell
type PercentCellProps = {
  value: number | null
  rounding: number
  className?: string
  opinion?: 'judgement' | 'neutral'
}

export const PercentCell = memo(function NumericCell({
  value,
  rounding,
  className,
  opinion = 'neutral',
}: PercentCellProps) {
  const isFilled = value !== null

  let fontColoring
  switch (opinion) {
    case 'judgement':
      fontColoring = !isFilled || value === 0 ? '' : value > 0 ? 'text-positive' : 'text-negative'
      break
    default:
      fontColoring = ''
  }

  return (
    <>
      <span
        className={cn(
          'table-text font-mono tracking-[0.05em]',
          !isFilled && 'text-tertiary-foreground',
          isFilled && fontColoring,
          className
        )}
      >
        {isFilled
          ? value.toLocaleString(undefined, {
              minimumFractionDigits: rounding,
              maximumFractionDigits: rounding,
            })
          : '—'}
      </span>
      <span
        className={cn(
          'table-text font-mono tracking-[0.05em]',
          !isFilled && 'text-tertiary-foreground',
          isFilled && fontColoring,
          className
        )}
      >
        %
      </span>
    </>
  )
})


type FooterCellProps = {
  value: string | null
  className?: string
}
export const FooterCell = memo(function FooterCell({ value, className }: FooterCellProps) {
  return <span className={cn('table-text text-tertiary-foreground', className)}>{value}</span>
})
