import { AlertCircle, Loader2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { getApiBaseUrl } from '@/utils/environment'
import { hardRedirect } from '@/utils/navigation'
import { consumeReturnTo } from '@/utils/returnTo'

/**
 * AuthCallback
 *
 * The identity provider redirects to the BACKEND /api/v1/auth/callback, which
 * sets the httpOnly session cookie and then 302-redirects the browser to the
 * frontend root ("/"). This component is therefore NOT part of the normal
 * sign-in path.
 *
 * It exists as a safety net for two scenarios:
 *   1. An `error` query param is present — the identity provider reported an error.
 *   2. The route is hit directly with code/state params (misconfigured redirect
 *      URI pointing here instead of the backend) — we redirect to the backend
 *      callback endpoint so it can complete the exchange.
 *
 * If neither condition applies (e.g., stale bookmark), we redirect to "/".
 */
export function AuthCallback() {
  const [searchParams] = useSearchParams()
  const [error, setError] = useState<string>('')
  const [isLoading, setIsLoading] = useState(true)
  const hasHandled = useRef(false)

  useEffect(() => {
    if (hasHandled.current) return
    hasHandled.current = true

    const code = searchParams.get('code')
    const state = searchParams.get('state')
    const errorParam = searchParams.get('error')

    if (errorParam) {
      setError('Authentication was cancelled or failed')
      setIsLoading(false)
      return
    }

    if (code && state) {
      // Proxy to backend callback: backend sets cookie and redirects to "/"
      const backendUrl = `${getApiBaseUrl()}/auth/callback?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`
      window.location.href = backendUrl
      // Keep loading spinner visible while the redirect happens
      return
    }

    // Nothing to do — send the user to their stashed return path, or home.
    // (In the normal flow the backend redirects to "/" and the in-app resume
    // handles return_to; this covers a direct/stale hit of this route.)
    hardRedirect(consumeReturnTo())
  }, [searchParams])

  if (isLoading) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center p-4">
        <div className="flex flex-col items-center gap-3 text-center">
          <Loader2 className="text-primary size-10 animate-spin" />
          <div>
            <h2 className="text-lg font-semibold">Signing you in…</h2>
            <p className="text-muted-foreground text-sm">
              Please wait while we complete your authentication.
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="bg-background flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardContent className="flex flex-col gap-4 p-6">
          <Alert variant="destructive">
            <AlertCircle className="size-4" />
            <AlertTitle>Authentication failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
          <Button
            onClick={() => {
              window.location.href = '/'
            }}
          >
            Try again
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
