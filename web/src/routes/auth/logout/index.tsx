import { useEffect, useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/authStore'
import { PageContainer } from '@/components/page-primitives/PageContainer'
import { PageStatusFooter } from '@/components/page-primitives/PageStatusFooter'
import { Button } from '@/components/ui/button'
import { ArrowUpRight } from '@phosphor-icons/react'
import { Spinner } from '@/components/ui/spinner'
import { clearAllApplicationData } from '@/lib/cache-utils'

export const Route = createFileRoute('/auth/logout/')({
  component: LogoutPage,
})

function LogoutPage() {
  const logout = useAuthStore((state) => state.logout)
  const [isResetting, setIsResetting] = useState(true)
  const [isCounting, setIsCounting] = useState(true)
  const navigate = useNavigate()
  const isReady = !isResetting && !isCounting

  useEffect(() => {
    const performReset = async () => {
      await logout()
      await clearAllApplicationData()
      setIsResetting(false)
    }
    performReset()
  }, [])

  useEffect(() => {
    const timer = setTimeout(() => setIsCounting(false), 5000)
    return () => clearTimeout(timer)
  }, [])

  return (
    <PageContainer variant="fullscreen">
      <div className="flex-1 flex h-0 items-center justify-center gap-4 text-center">
        <div className="flex flex-col gap-4 items-center justify-stretch max-w-xl px-4">
          <span className="text-[11px] leading-[16px] font-medium font-sans tracking-[0.03em] text-tertiary-foreground">
            &nbsp;
          </span>
          <span className="text-[2rem] leading-[46px] font-medium font-display tracking-tight">
            {isReady ? "You've Been Logged Out" : 'Logging Out...'}
          </span>
          <p className="text-[14px] leading-[24px] text-tertiary-foreground">
            {isReady ? 'See you next time!' : 'This may take a few seconds'}
          </p>
          <div className="mt-4 h-12 flex items-center justify-center">
            {isReady ? (
              <Button
                variant="defaultInverted"
                size={'lg'}
                className={'gap-10'}
                onClick={() => navigate({ to: '/' })}
              >
                <span>TAKE ME HOME</span>
                <ArrowUpRight className={'size-4'} />
              </Button>
            ) : (
              <Spinner
                className={'w-80 h-[2px] bg-primary-foreground/20 before:bg-primary-foreground'}
              />
            )}
          </div>
        </div>
      </div>
      <PageStatusFooter message={'LOGOUT'} />
    </PageContainer>
  )
}
