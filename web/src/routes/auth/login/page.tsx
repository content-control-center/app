import { Link } from '@tanstack/react-router'
import { AppAuth } from '@/components/layout/AppAuth'
import { AuthLoginForm } from '@/components/forms/authLoginForm'

function LoginPage() {
  return (
    <AppAuth
      title="Welcome Back"
      subtitle="Log in to continue managing your investments"
      form={<AuthLoginForm />}
      bottomNav={
        <>
          Don't have an account?{' '}
          <Link to="/auth/register" className={'text-primary-foreground font-medium'}>
            Sign up here
          </Link>
        </>
      }
    />
  )
}

export default LoginPage
