import { AppAuth } from '@/components/layout/AppAuth'
import { AuthLoginForm } from '@/components/forms/authLoginForm'

function LoginPage() {
  return (
    <AppAuth
      title="Welcome Back"
      subtitle="Log in to continue managing your content"
      form={<AuthLoginForm />}
      bottomNav={
        <>
          Issues with getting access? Contact the instance owner
        </>
      }
    />
  )
}

export default LoginPage
