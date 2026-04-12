import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/icon'
import { useResendVerificationEmail } from '@/hooks/useAuth'

const RESEND_COOLDOWN_SECONDS = 60

type AuthEmailSentProps = {
  email: string
}

export function AuthEmailSent({ email }: AuthEmailSentProps) {
  const { mutate: resend, isPending } = useResendVerificationEmail()
  const [cooldown, setCooldown] = useState(0)

  useEffect(() => {
    if (cooldown <= 0) return

    const timer = setTimeout(() => setCooldown((c) => c - 1), 1000)
    return () => clearTimeout(timer)
  }, [cooldown])

  const handleResend = useCallback(() => {
    resend(email, {
      onSuccess: () => setCooldown(RESEND_COOLDOWN_SECONDS),
    })
  }, [email, resend])

  const isDisabled = isPending || cooldown > 0

  return (
    <div className="flex flex-col gap-4 h-[432px] animate-in fade-in duration-500">
      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium">
          Click the link in the email to verify your account and get started.
        </span>
      </div>
      <div className="h-[226px]"></div>
      <div className="text-[13px] leading-4 text-secondary-foreground">
        <span>
          Wrong email?{' '}
          <a href="mailto:support@quantmanager.io" className="text-primary-foreground font-medium">
            Contact our support team
          </a>
        </span>
      </div>

      <div className="text-[13px] leading-4 text-secondary-foreground">
        <span>
          Cannot find the email? Check your spam folder or{' '}
          <button
            type="button"
            onClick={handleResend}
            disabled={isDisabled}
            className="text-primary-foreground font-medium disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {cooldown > 0 ? `resend in ${cooldown}s` : 'resend it'}
          </button>
        </span>
      </div>

      <div className="w-full">
        <Button
          type="button"
          variant="default"
          size="default"
          className="w-full justify-between bg-secondary"
          disabled
        >
          <span>VERIFICATION EMAIL SENT</span>
          <Icon className="size-4" name="check" />
        </Button>
      </div>
      <div></div>
    </div>
  )
}
