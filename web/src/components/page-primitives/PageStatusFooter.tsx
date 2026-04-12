export type PageStatusFooterProps = {
  message?: string
}
export function PageStatusFooter({ message = 'ERROR' }: PageStatusFooterProps) {
  return (
    <div
      className={
        'relative flex-shrink-0 h-[300px] bottom-0 left-0 right-0 p-y-2 flex flex-col gap-0 overflow-hidden'
      }
    >
      <div className={'flex h-[140px] gap-24 justify-center items-center relative'}>
        <div
          className={
            'font-display text-senary-foreground text-center text-[140px] leading-none pt-2'
          }
        >
          {message}
        </div>
        <div
          className={
            'font-display text-senary-foreground text-center text-[140px] leading-none pt-2'
          }
        >
          {message}
        </div>
        <div
          className={
            'font-display text-senary-foreground text-center text-[140px] leading-none pt-2'
          }
        >
          {message}
        </div>
        <div className={'absolute inset-0 background-fader-2'} />
      </div>
      <div className={'h-40 relative'}>
        <div className={'h-full bg-[url(/textures/ticks_tile.png)] bg-size-[9px] opacity-80'} />
        <div className={'absolute inset-0 background-fader-1'} />
      </div>
    </div>
  )
}
