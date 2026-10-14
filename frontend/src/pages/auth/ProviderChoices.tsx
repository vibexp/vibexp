import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useAuth } from '@/contexts/useAuth'
import type { AuthProvider } from '@/services/authService'
import { sessionStore } from '@/utils/storage'

/**
 * "Continue with …" for each enabled sign-in provider, on a public page that
 * creates an account (#1440). `beforeSignIn` runs first, so the page can stash
 * what the round trip must come back to.
 */
export function ProviderChoices({
  providers,
  beforeSignIn,
  disabled,
}: Readonly<{
  providers: readonly AuthProvider[]
  beforeSignIn?: () => void
  disabled?: boolean
}>) {
  const { login } = useAuth()
  const [starting, setStarting] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  if (providers.length === 0) return null

  const start = async (provider: AuthProvider) => {
    setError(null)
    setStarting(provider.name)
    try {
      beforeSignIn?.()
      sessionStore.set(STORAGE_KEYS.LOGIN_METHOD, provider.display_name)
      await login(provider.name)
    } catch (err) {
      setError(
        err instanceof Error && err.message
          ? err.message
          : `Signing in with ${provider.display_name} could not start`
      )
      setStarting(null)
    }
  }

  return (
    <div className="space-y-2" data-testid="provider-choices">
      {providers.map(provider => (
        <Button
          key={provider.name}
          type="button"
          variant="outline"
          className="w-full"
          disabled={(disabled ?? false) || starting !== null}
          onClick={() => {
            void start(provider)
          }}
        >
          {starting === provider.name
            ? 'Redirecting…'
            : `Continue with ${provider.display_name}`}
        </Button>
      ))}
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      )}
    </div>
  )
}
