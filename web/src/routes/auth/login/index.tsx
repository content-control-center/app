import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'
import LoginPage from './page'

const loginSearchSchema = z.object({
  redirect: z.string().optional(),
})

export const Route = createFileRoute('/auth/login/')({
  validateSearch: loginSearchSchema,
  beforeLoad: ({ context }) => {
    if (context.auth.isAuthenticated()) {
      throw redirect({ to: '/' })
    }
  },
  component: LoginPage,
})
