import { PageStatusFooter } from '@/components/page-primitives/PageStatusFooter.tsx'
import type { ReactNode } from 'react'

type PageErrorProps = {
  children?: ReactNode
  header?: string
  subHeader?: string
  message?: ReactNode
  action?: ReactNode
  errorType?: string
}

export function PageError({
  children,
  subHeader = 'ERROR',
  header = 'Something went wrong',
  message = (
    <>
      An error has occurred
      <br />
      Please try this once more
    </>
  ),
  action,
  errorType = 'ERROR',
}: PageErrorProps) {
  return (
    <div className={'h-full w-full overflow-hidden flex flex-col items-stretch gap-0'}>
      <div className="flex-1 flex h-0 items-center justify-center gap-4">
        {children ?? (
          <div className="flex flex-col gap-4 items-center justify-stretch max-w-xl px-4">
            <span className="text-[11px] leading-4 font-medium font-sans tracking-[0.03em] text-tertiary-foreground">
              {subHeader}
            </span>
            <span className="text-[2rem] leading-[46px] font-medium font-display tracking-tight">
              {header}
            </span>
            <p className="text-[14px] leading-6 text-tertiary-foreground">{message}</p>
            {action && <div className="mt-4">{action}</div>}
          </div>
        )}
      </div>
      <PageStatusFooter message={errorType} />
    </div>
  )
}
