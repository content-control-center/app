import { type ReactNode } from 'react'
import LogoFull from '@/assets/logo-full.svg?react'
import { Link } from '@tanstack/react-router'

type AppAuthProps = {
  title?: string
  subtitle?: string
  form: ReactNode
  bottomNav: ReactNode | undefined
}

export function AppAuth({ title, subtitle, form, bottomNav }: AppAuthProps) {
  return (
    <div className="min-h-screen min-w-screen bg-background relative">
      <div className="absolute inset-0 md:inset-12 xl:top-12 xl:bottom-12">
        <div className="w-full md:max-w-[528px] xl:w-[528px] mx-auto h-full bg-primary px-6 py-8 md:px-10 md:py-12 flex flex-col">
          <div className="flex h-0 flex-1 flex-col gap-4 md:gap-10 overflow-y-auto">
            <div className="shrink-0">
              <Link to={'/'}>
                <LogoFull className={'h-6 w-auto md:h-8'} />
              </Link>
            </div>
            <div className="flex-1 flex flex-col justify-center">
              <div className={'flex flex-col gap-6'}>
                {title && (
                  <div>
                    <h1
                      className={
                        'text-[1.5rem] leading-10 md:text-[2rem] md:leading-12 font-medium font-display tracking-tight'
                      }
                    >
                      {title}
                    </h1>
                    {subtitle && (
                      <div className="hidden md:block text-[13px] pt-1 leading-4 text-secondary-foreground">
                        {subtitle}
                      </div>
                    )}
                  </div>
                )}
                {form}
              </div>
            </div>
            {bottomNav && (<div className="shrink-0 mt-auto text-[13px] leading-4 text-secondary-foreground">
              {bottomNav}
            </div>)}
          </div>
        </div>
      </div>
    </div>
  )
}
