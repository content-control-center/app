import { type FormEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ArrowUpRight } from '@phosphor-icons/react'
import { useRegister } from '@/hooks/useAuth'
import { useFormValidation } from '@/hooks/useFormValidation'
import { registerSchema, PASSWORD_RULES, cn } from '@/lib'
import type { LoginPayload } from '@/types/session'

type AuthRegisterFormProps = {
  onSuccess?: (credentials: LoginPayload) => void
}

export function AuthRegisterForm({ onSuccess }: AuthRegisterFormProps) {
  const navigate = useNavigate()
  const { mutate: register, isPending, error } = useRegister()
  const { values, setField, fieldErrors, validate } = useFormValidation(registerSchema, {
    firstName: '',
    lastName: '',
    email: '',
    password: '',
  })

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()

    const data = validate()
    if (!data) return

    register(data, {
      onSuccess: () => {
        if (onSuccess) {
          onSuccess({ email: data.email, password: data.password })
        } else {
          navigate({ to: '/' })
        }
      },
    })
  }

  return (
    <>
      <form
        onSubmit={handleSubmit}
        className="flex flex-col gap-4 shrink-0 animate-in fade-in duration-500"
        noValidate
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="firstName">First Name</Label>
          <Input
            id="firstName"
            type="text"
            autoComplete="given-name"
            variant="default"
            placeholder="Enter your first name"
            value={values.firstName}
            onChange={(e) => setField('firstName', e.target.value)}
            aria-invalid={!!fieldErrors.firstName}
            disabled={isPending}
          />
          {fieldErrors.firstName && (
            <p className="text-xs text-destructive">{fieldErrors.firstName}</p>
          )}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="lastName">Last Name</Label>
          <Input
            id="lastName"
            type="text"
            autoComplete="family-name"
            variant="default"
            placeholder="Enter your last name"
            value={values.lastName}
            onChange={(e) => setField('lastName', e.target.value)}
            aria-invalid={!!fieldErrors.lastName}
            disabled={isPending}
          />
          {fieldErrors.lastName && (
            <p className="text-xs text-destructive">{fieldErrors.lastName}</p>
          )}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            autoComplete="username"
            variant="default"
            placeholder="Enter your email"
            value={values.email}
            onChange={(e) => setField('email', e.target.value)}
            aria-invalid={!!fieldErrors.email}
            disabled={isPending}
          />
          {fieldErrors.email && <p className="text-xs text-destructive">{fieldErrors.email}</p>}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            variant="default"
            placeholder="Enter password"
            value={values.password}
            onChange={(e) => setField('password', e.target.value)}
            aria-invalid={!!fieldErrors.password}
            disabled={isPending}
          />
          {(() => {
            const allPassed = PASSWORD_RULES.every(({ test }) => test(values.password))
            return (
              <p className={`text-xs ${allPassed ? 'text-positive' : 'text-tertiary-foreground'}`}>
                {PASSWORD_RULES.map(({ test, label }, i) => {
                  const passed = test(values.password)
                  const isLast = i === PASSWORD_RULES.length - 1
                  return (
                    <span key={label}>
                      <span
                        className={`text-xs ${allPassed ? 'text-positive' : passed ? 'text-positive' : 'text-tertiary-foreground'}`}
                      >
                        {isLast ? 'and ' : ''}
                        {label}
                        {isLast ? '' : ', '}
                      </span>
                    </span>
                  )
                })}
                {allPassed && '  \u2713'}
              </p>
            )
          })()}
        </div>
        <div className="w-full">
          <Button
            type="submit"
            variant="defaultInverted"
            size="default"
            className={'w-full justify-between'}
            loading={isPending}
            disabled={isPending}
          >
            <span>SIGN UP</span>
            <ArrowUpRight className="size-4" />
          </Button>
          <div className="h-4 my-4">
            <span
              className={cn(
                'text-sm text-destructive transition-opacity duration-300',
                error ? ' opacity-100' : 'opacity-0'
              )}
            >
              {error && error.message}
            </span>
          </div>
        </div>
      </form>
    </>
  )
}
