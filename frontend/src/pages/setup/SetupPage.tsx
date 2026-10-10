import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { SITE_NAME } from '@/config/siteConfig'
import { AllowlistSection } from '@/pages/admin/settings/auth/AllowlistSection'
import { ProvidersSection } from '@/pages/admin/settings/auth/ProvidersSection'
import { useAuthProviders } from '@/pages/admin/settings/auth/useAuthProviders'
import { authService } from '@/services/authService'
import {
  type AdminAuthProvider,
  authSettingsService,
} from '@/services/authSettingsService'
import { setupService } from '@/services/setupService'
import { ApiError } from '@/types/errors'

/**
 * Where the token exchange stands. `invalid` is a 401 (a wrong, used-up or
 * expired token), `inactive` a 404 (the instance is not in setup mode).
 */
type SetupState =
  | { status: 'exchanging' }
  | { status: 'ready' }
  | { status: 'missing' | 'invalid' | 'inactive' }
  | { status: 'failed'; message: string }

const TERMINAL_COPY: Record<
  'missing' | 'invalid' | 'inactive',
  { title: string; body: string }
> = {
  missing: {
    title: 'This setup link is incomplete',
    body: 'There is no setup token in this address and no setup session to resume. Open the full SETUP URL from the server logs, including its token.',
  },
  invalid: {
    title: 'This setup link is no longer valid',
    body: 'The token is wrong, was already used to finish setup, or has expired. Run `vibexp admin auth setup rearm` on the server for a new setup URL, or restart the server once the old token has expired.',
  },
  inactive: {
    title: 'This instance is already set up',
    body: 'Setup mode is off because a sign-in provider is enabled. Sign in and change the authentication settings under Admin → Settings → Authentication.',
  },
}

function exchangeFailure(err: unknown): SetupState {
  if (err instanceof ApiError && err.status === 401)
    return { status: 'invalid' }
  if (err instanceof ApiError && err.status === 404) {
    return { status: 'inactive' }
  }
  return {
    status: 'failed',
    message:
      err instanceof Error ? err.message : 'Failed to start the setup session',
  }
}

function SetupFrame({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className="bg-background text-foreground min-h-screen">
      <header className="border-b px-4 py-3 md:px-6">
        <span className="text-base font-bold tracking-tight">{SITE_NAME}</span>
        <span className="text-muted-foreground ml-2 text-sm">Setup</span>
      </header>
      <main className="mx-auto w-full max-w-screen-lg space-y-6 px-4 py-6 md:px-6">
        {children}
      </main>
    </div>
  )
}

/**
 * The last step of setup: once a provider is enabled, sign in through it as a
 * root admin, which is what ends setup mode. A provider that is enabled but
 * could not be built is not offered for sign-in, so it is called out instead.
 */
export function SetupFinishPrompt({
  providers,
}: Readonly<{ providers: readonly AdminAuthProvider[] }>) {
  const [signingIn, setSigningIn] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const enabled = providers.filter(provider => provider.enabled)
  const usable = enabled.filter(
    provider => provider.health.status !== 'unhealthy'
  )

  if (enabled.length === 0) {
    return (
      <Alert data-testid="setup-next-step">
        <AlertTitle>Next: add and enable a sign-in provider</AlertTitle>
        <AlertDescription>
          Setup finishes when a root admin signs in through one.
        </AlertDescription>
      </Alert>
    )
  }

  const signIn = async (provider: AdminAuthProvider) => {
    try {
      setSigningIn(provider.slug)
      setError(null)
      // GET /api/v1/auth/login?provider=<slug> answers with the identity
      // provider's URL rather than redirecting, so follow it here.
      globalThis.location.assign(await authService.getLoginUrl(provider.slug))
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : `Failed to sign in with ${provider.display_name}`
      )
      setSigningIn(null)
    }
  }

  return (
    <Card data-testid="setup-finish">
      <CardHeader>
        <CardTitle>Finish setup</CardTitle>
        <CardDescription>
          Setup mode ends when a root admin (an address in the server&apos;s
          instance admin configuration) signs in through an enabled provider.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {usable.length === 0 && (
          <Alert variant="destructive" data-testid="setup-unhealthy">
            <AlertTitle>No enabled provider can be used yet</AlertTitle>
            <AlertDescription>
              Every enabled provider failed to build, so none is offered for
              sign-in. Fix its configuration above, then sign in.
            </AlertDescription>
          </Alert>
        )}
        {usable.map(provider => (
          <Button
            key={provider.id}
            type="button"
            disabled={signingIn !== null}
            data-testid="setup-sign-in"
            data-provider-slug={provider.slug}
            onClick={() => {
              void signIn(provider)
            }}
          >
            {signingIn === provider.slug
              ? 'Redirecting…'
              : `Sign in with ${provider.display_name} as a root admin to finish setup`}
          </Button>
        ))}
        {error && (
          <Alert variant="destructive" data-testid="setup-sign-in-error">
            <AlertTitle>Sign in error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  )
}

/**
 * What a setup session can reach: the sign-in providers and the access
 * allowlist. The instance admins and the audit history are admin-only (a 404
 * on a setup session), so they are not rendered here.
 */
function SetupSections() {
  const [providersKey, setProvidersKey] = useState(0)
  const providers = useAuthProviders(providersKey)
  return (
    <>
      <div className="space-y-1">
        <h1 className="text-2xl font-bold tracking-tight">
          Set up sign-in for this instance
        </h1>
        <p className="text-muted-foreground text-sm">
          Add at least one sign-in provider, optionally restrict who may sign
          in, then sign in as a root admin. This setup session lasts up to an
          hour.
        </p>
      </div>
      <ProvidersSection state={providers} />
      <AllowlistSection
        onSaved={() => {
          setProvidersKey(key => key + 1)
        }}
      />
      {providers.providers && (
        <SetupFinishPrompt providers={providers.providers} />
      )}
    </>
  )
}

/**
 * First-run setup, `/setup?token=…` (#1239, epic #1230). A fresh instance has
 * no sign-in provider, so nobody can sign in to configure one: the server logs
 * a one-time setup URL instead, and this page exchanges its token for a setup
 * session (the `vibexp_setup` cookie) that can manage the providers and the
 * allowlist — nothing else.
 *
 * The token is removed from the address bar as soon as it is read, so it does
 * not linger in the history or get copied along with the URL; a reload then
 * has no token and resumes on the cookie while it is valid. There is no user
 * on a setup session, so the page mounts under no auth gate and no team or
 * project provider.
 */
export function SetupPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  // Read once: the token is stripped from the URL below.
  const [token] = useState(() => searchParams.get('token') ?? '')
  const [state, setState] = useState<SetupState>({ status: 'exchanging' })

  useEffect(() => {
    let active = true
    if (token.length === 0) {
      // No token: a reload of this page after the token was stripped. The
      // setup cookie may still be valid, which the first setup call tells.
      authSettingsService
        .listProviders()
        .then(() => {
          if (active) setState({ status: 'ready' })
        })
        .catch(() => {
          if (active) setState({ status: 'missing' })
        })
      return () => {
        active = false
      }
    }
    setSearchParams({}, { replace: true })
    setupService
      .createSession(token)
      .then(() => {
        if (active) setState({ status: 'ready' })
      })
      .catch((err: unknown) => {
        if (active) setState(exchangeFailure(err))
      })
    return () => {
      active = false
    }
  }, [setSearchParams, token])

  if (state.status === 'exchanging') {
    return (
      <SetupFrame>
        <div className="flex justify-center py-12" data-testid="setup-loading">
          <LoadingSpinner size="lg" />
        </div>
      </SetupFrame>
    )
  }

  if (state.status === 'ready') {
    return (
      <SetupFrame>
        <SetupSections />
      </SetupFrame>
    )
  }

  if (state.status === 'failed') {
    return (
      <SetupFrame>
        <Alert variant="destructive" data-testid="setup-failed">
          <AlertTitle>Setup could not start</AlertTitle>
          <AlertDescription>{state.message}</AlertDescription>
        </Alert>
      </SetupFrame>
    )
  }

  const copy = TERMINAL_COPY[state.status]
  return (
    <SetupFrame>
      <Alert variant="destructive" data-testid={`setup-${state.status}`}>
        <AlertTitle>{copy.title}</AlertTitle>
        <AlertDescription>{copy.body}</AlertDescription>
      </Alert>
    </SetupFrame>
  )
}
