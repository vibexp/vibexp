import { AlertCircle, Check, Loader2, Shield, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { oauthService } from '@/services/oauthService'
import { ApiError } from '@/types/errors'
import type { OAuthConsentAction, OAuthConsentDetails } from '@/types/oauth'
import { hardRedirect } from '@/utils/navigation'

const MISSING_LOGIN_ERROR =
  'This authorization link is missing required information. Start the connection again from your client.'
const LOAD_ERROR =
  'This authorization request has expired or is no longer valid. Start the connection again from your client.'
const DECISION_ERROR =
  'We could not complete the authorization. Start the connection again from your client.'

/**
 * OAuthConsentPage renders the embedded Authorization Server's consent screen
 * (issue #52). It is a public route (outside AuthGate), reached with an opaque
 * `login` id after /oauth2/authorize.
 *
 * The AS never authenticates anyone itself (issue #54): the login session has no
 * user until the SPA binds the logged-in app user. So this page gates on login
 * (#55): it fetches the request details and, when no user is bound yet
 * (`authenticated: false`), tries to attach the current app session. If that
 * succeeds the approval screen renders; if it fails with 401 the visitor is sent
 * to the app login page with a `return_to` back here, so after logging in they
 * land on the same consent URL. Approve/deny then posts the decision and
 * navigates the browser to the URL the backend returns so the OAuth client (e.g.
 * Claude Code) receives the code. No OAuth secret ever reaches the SPA.
 */
export function OAuthConsentPage() {
  const [searchParams] = useSearchParams()
  const login = searchParams.get('login') ?? ''

  const [details, setDetails] = useState<OAuthConsentDetails | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [processing, setProcessing] = useState(false)

  useEffect(() => {
    let cancelled = false

    const load = async () => {
      if (!login) {
        setError(MISSING_LOGIN_ERROR)
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        let data = await oauthService.getConsent(login)
        // No app user bound to the AS login session yet: bind the current
        // session, or send the visitor to log in and return here.
        if (!data.authenticated) {
          try {
            await oauthService.attach(login, data.csrf)
          } catch (attachErr) {
            if (attachErr instanceof ApiError && attachErr.status === 401) {
              // Signed out: go to the login page and come back to this exact
              // consent URL. Keep the spinner up while the browser navigates.
              const consentUrl = `/oauth/consent?login=${encodeURIComponent(login)}`
              hardRedirect(`/login?return_to=${encodeURIComponent(consentUrl)}`)
              return
            }
            throw attachErr
          }
          // Now bound — re-fetch the (now authenticated) approval details.
          data = await oauthService.getConsent(login)
        }
        if (!cancelled) {
          setDetails(data)
          setLoading(false)
        }
      } catch (err) {
        console.error('Failed to load consent request:', err)
        if (!cancelled) {
          setError(LOAD_ERROR)
          setLoading(false)
        }
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [login])

  const decide = async (action: OAuthConsentAction) => {
    if (!details) return
    try {
      setProcessing(true)
      const { redirect_to } = await oauthService.submitConsent(
        login,
        details.csrf,
        action
      )
      // Hand the browser to the OAuth client's callback (code or error).
      hardRedirect(redirect_to)
    } catch (err) {
      console.error('Failed to submit consent decision:', err)
      setError(DECISION_ERROR)
      setProcessing(false)
    }
  }

  if (loading) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center p-4">
        <div className="flex flex-col items-center gap-3 text-center">
          <Loader2 className="text-primary size-10 animate-spin" />
          <p className="text-muted-foreground text-sm">
            Loading authorization request…
          </p>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardContent className="p-6">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertTitle>Authorization unavailable</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (!details) {
    return null
  }

  return (
    <div className="bg-background flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-lg">
        <CardHeader className="text-center">
          <div className="bg-primary text-primary-foreground mx-auto mb-2 flex size-12 items-center justify-center rounded-full">
            <Shield className="size-6" />
          </div>
          <CardTitle>Authorize access</CardTitle>
          <CardDescription>
            <span className="font-medium">{details.client_name}</span> is
            requesting access to your VibeXP account.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="bg-muted/40 space-y-3 rounded-lg border p-4">
            <InfoRow label="Application">
              <span className="font-medium">{details.client_name}</span>
            </InfoRow>
            {details.redirect_host && (
              <>
                <Separator />
                <InfoRow label="Redirects to">
                  <code className="text-xs">{details.redirect_host}</code>
                </InfoRow>
              </>
            )}
            {details.scopes && details.scopes.length > 0 && (
              <>
                <Separator />
                <InfoRow label="Requested access">
                  <ul className="space-y-0.5">
                    {details.scopes.map(scope => (
                      <li key={scope} className="font-medium">
                        {scope}
                      </li>
                    ))}
                  </ul>
                </InfoRow>
              </>
            )}
          </div>

          <div className="flex gap-2">
            <Button
              className="flex-1"
              disabled={processing}
              onClick={() => {
                void decide('approve')
              }}
            >
              <Check className="mr-2 size-4" />
              {processing ? 'Working…' : 'Approve'}
            </Button>
            <Button
              variant="outline"
              className="flex-1"
              disabled={processing}
              onClick={() => {
                void decide('deny')
              }}
            >
              <X className="mr-2 size-4" />
              {processing ? 'Working…' : 'Deny'}
            </Button>
          </div>

          <p className="text-muted-foreground text-center text-xs">
            Approving lets this application access your VibeXP account on your
            behalf. You can revoke access at any time.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

function InfoRow({
  label,
  children,
}: Readonly<{
  label: string
  children: React.ReactNode
}>) {
  return (
    <div className="flex items-start justify-between gap-4 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <div className="text-right">{children}</div>
    </div>
  )
}
