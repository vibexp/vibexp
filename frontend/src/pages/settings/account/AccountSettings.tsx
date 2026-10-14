import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/useAuth'

import { ProfileCard } from './ProfileCard'
import { SignInMethodsCard } from './SignInMethodsCard'
import { useAccount } from './useAccount'

/**
 * Settings → Account (#1441, epic #1426): the signed-in person's display name
 * and the ways they sign in, over the self-service account API (#1437).
 *
 * `leave` is injectable so a test does not navigate.
 */
export function AccountSettings({
  leave,
}: Readonly<{ leave?: (url: string) => void }> = {}) {
  const state = useAccount(leave)
  const { refreshUser } = useAuth()
  const { account, loadError } = state

  return (
    <div className="space-y-6">
      <PageHeader
        title="Account settings"
        description="Your name, your password and the ways you sign in."
      />

      {loadError && (
        <Alert variant="destructive" data-testid="account-load-error">
          <AlertTitle>Your account could not be loaded</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{loadError}</p>
            <Button
              variant="outline"
              size="sm"
              disabled={state.busy}
              onClick={() => {
                void state.reload()
              }}
            >
              Try again
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {!account && !loadError && (
        // Also while a failed load is being retried.
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      )}

      {account && (
        <>
          <ProfileCard
            state={state}
            account={account}
            refreshUser={refreshUser}
          />
          <SignInMethodsCard state={state} account={account} />
        </>
      )}
    </div>
  )
}
