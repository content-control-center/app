import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { AppAuth } from '@/components/layout/AppAuth'

type PageState = 'form' | 'email-sent'

function RegisterPage() {
  const [pageState] = useState<PageState>('form')
  const [registeredEmail] = useState<string>()

  const getTitleSubtitle = (): { title: string; subtitle: string } => {
    switch (pageState) {
      case 'form':
        return {
          title: 'Create an Account',
          subtitle: 'Sign up to start building clarity in your investments',
        }
      case 'email-sent':
        return {
          title: 'Check Your Inbox',
          subtitle: `We've sent a verification link to ${registeredEmail}`,
        }
    }
  }

  const renderForm = () => {
    switch (pageState) {
      case 'form':
        return (
          <div>form</div>
        )
      case 'email-sent':
        return   <div>sent</div>
    }
  }

  const { title, subtitle } = getTitleSubtitle()

  return (
    <AppAuth
      title={title}
      subtitle={subtitle}
      form={renderForm()}
      bottomNav={
        <>
          Already have an account?{' '}
          <Link to="/auth/login" className={'text-primary-foreground font-medium'}>
            Log in here
          </Link>
        </>
      }
    />
  )
}

export default RegisterPage
