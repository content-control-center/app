import { type FormEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ArrowUpRight } from '@phosphor-icons/react'
import { useLogin } from '@/hooks/useAuth'
import { useFormValidation } from '@/hooks/useFormValidation'
import { loginSchema, cn } from '@/lib'

export function AuthLoginForm() {
  const navigate = useNavigate()
  const { mutate: login, isPending, error, reset } = useLogin()
  const { values, setField, fieldErrors, validate } = useFormValidation(loginSchema, {
    email: '',
    password: '',
  })

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()

    const data = validate()
    if (!data) return

    login(data, {
      onSuccess: () => {
        navigate({ to: '/' })
      },
    })
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-col gap-4 shrink-0 animate-in fade-in duration-500"
      noValidate
    >
      <div className="flex flex-col gap-2">
        <Label htmlFor="email">Email</Label>
        <Input
          id="email"
          type="email"
          variant="default"
          placeholder="Enter your email"
          value={values.email}
          onChange={(e) => {
            setField('email', e.target.value)
            if (error) reset()
          }}
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
          variant="default"
          placeholder="Enter password"
          value={values.password}
          onChange={(e) => {
            setField('password', e.target.value)
            if (error) reset()
          }}
          aria-invalid={!!fieldErrors.password}
          disabled={isPending}
        />
        {fieldErrors.password ? (
          <p className="text-xs text-destructive">{fieldErrors.password}</p>
        ) : (
          <p className="text-xs text-tertiary-foreground">
            Min. 8 chars, an uppercase, a lowercase, and a digit
          </p>
        )}
      </div>
      <div></div>
      <div className="w-full">
        <Button
          type="submit"
          variant="defaultInverted"
          size="default"
          className={'w-full justify-between'}
          loading={isPending}
          disabled={isPending}
        >
          <span>LOG IN</span>
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
  )
}
