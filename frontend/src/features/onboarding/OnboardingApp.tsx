import { AuthGate } from '@/components/auth/AuthGate'
import { Root } from '@/components/layout/Root'
import { Toaster } from '@/components/ui/sonner'

import { GettingStartedPage } from './GettingStartedPage'

/**
 * The first-run onboarding's branch of the app: signed in, full-screen with
 * its own header, so outside the app shell and its team/project providers.
 */
export function OnboardingApp() {
  return (
    <Root>
      <AuthGate>
        <GettingStartedPage />
      </AuthGate>
      <Toaster />
    </Root>
  )
}
