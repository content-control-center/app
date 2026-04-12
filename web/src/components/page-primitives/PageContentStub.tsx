type PageStubProps = {
  title?: string
  header?: string
  messageString?: string
}

export function PageContentStub({ header, title, messageString }: PageStubProps) {

  return (
    <div className="relative h-full">
      {/* Stub content */}
      <div
        className={
          'absolute inset-0 flex flex-col items-center justify-center gap-10 transition-opacity duration-300'}
      >
        <div className="flex flex-col items-center justify-center gap-4">
          <span className="text-[11px] leading-4 font-medium font-sans tracking-[0.03em] text-tertiary-foreground">
            {title ?? 'STUB'}
          </span>
          <span className="text-[2rem] leading-[46px] font-medium font-display tracking-tight">
            {header ?? 'This module is not shipped yet'}
          </span>
          <p className="text-[16px] leading-6 text-center text-foreground">
            {messageString ??
              'While our team is busy developing it, you may test your ticker symbol knowledge'}
          </p>
        </div>
      </div>

    </div>
  )
}
