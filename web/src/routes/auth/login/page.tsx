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
          Don't have an account? Contact the instance owner
        </>
      }
    />
  )
}

export default LoginPage
