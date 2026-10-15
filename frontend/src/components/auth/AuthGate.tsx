import type { ReactNode } from 'react'

import { SignInPage } from '@/components/auth/SignInPage'
import { useAuth } from '@/contexts/useAuth'

/**
 * Renders its children only for a signed-in user: a loading line while the
 * session is being checked, the sign-in page when there is none. Shared by
 * every signed-in branch of the app (the team app, the admin portal, the
 * first-run onboarding).
 */
export function AuthGate({ children }: Readonly<{ children: ReactNode }>) {
  const { isAuthenticated, isLoading } = useAuth()

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-muted-foreground text-sm">Loading…</div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return <SignInPage />
  }

  return <>{children}</>
}
