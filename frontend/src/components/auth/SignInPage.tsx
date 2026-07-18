import { KeyRound, Moon, Shield, Sun, Users, Zap } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

import { CookieConsentBanner } from '@/components/CookieConsentBanner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/lib/theme'

import {
  PRIVACY_URL,
  SITE_DOMAIN,
  SITE_LEGAL_NAME,
  SITE_NAME,
  SITE_URL,
  SUPPORT_EMAIL,
  TERMS_URL,
} from '../../config/siteConfig'
import { STORAGE_KEYS } from '../../constants/storageKeys'
import { useAuth } from '../../contexts/AuthContext'
import { useAnalytics } from '../../hooks/useAnalytics'
import type { AuthProvider } from '../../services/authService'
import { authService } from '../../services/authService'
import { sanitizeReturnTo, stashReturnTo } from '../../utils/returnTo'
import { sessionStore } from '../../utils/storage'
import { DevLogin } from './DevLogin'

const CURRENT_YEAR = new Date().getFullYear()

function GoogleIcon({ className }: Readonly<{ className?: string }>) {
  return (
    <svg
      className={className}
      width="18"
      height="18"
      viewBox="0 0 18 18"
      aria-hidden="true"
    >
      <path
        d="M16.51 8H8.98v3h4.3c-.18 1-.74 1.48-1.6 2.04v2.01h2.6a7.8 7.8 0 002.38-5.88c0-.57-.05-.66-.15-1.18z"
        fill="#4285F4"
      />
      <path
        d="M8.98 17c2.16 0 3.97-.72 5.3-1.94l-2.6-2.04a4.8 4.8 0 01-7.18-2.54H1.83v2.07A8 8 0 008.98 17z"
        fill="#34A853"
      />
      <path
        d="M4.5 10.48a4.8 4.8 0 010-3.05V5.36H1.83a8 8 0 000 7.18l2.67-2.06z"
        fill="#FBBC05"
      />
      <path
        d="M8.98 4.18c1.17 0 2.23.4 3.06 1.2l2.3-2.3A8 8 0 001.83 5.36L4.5 7.43a4.77 4.77 0 014.48-3.25z"
        fill="#EA4335"
      />
    </svg>
  )
}

function GitHubIcon({ className }: Readonly<{ className?: string }>) {
  return (
    <svg
      className={className}
      width="18"
      height="18"
      viewBox="0 0 16 16"
      fill="currentColor"
      aria-hidden="true"
    >
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  )
}

// ProviderIcon renders a recognizable mark per known provider, falling back to
// a generic key icon so a newly-added or generic (OIDC) provider still renders.
function ProviderIcon({ name }: Readonly<{ name: string }>) {
  if (name === 'google') return <GoogleIcon />
  if (name === 'github') return <GitHubIcon />
  return <KeyRound className="h-4 w-4" />
}

function VibeXPMark() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M22 12h-4l-3 9L9 3l-3 9H2" />
    </svg>
  )
}

function Brand() {
  return (
    <div className="inline-flex items-center gap-2.5">
      <div className="bg-primary text-primary-foreground grid h-9 w-9 place-items-center rounded-lg">
        <VibeXPMark />
      </div>
      <span className="text-base font-bold tracking-tight">{SITE_NAME}</span>
    </div>
  )
}

function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()
  const isDark = resolvedTheme === 'dark'
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={() => {
        setTheme(isDark ? 'light' : 'dark')
      }}
      className="text-muted-foreground hover:text-foreground h-9 w-9"
      aria-label={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
      title={isDark ? 'Light mode' : 'Dark mode'}
    >
      {isDark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
    </Button>
  )
}

const PITCH_BULLETS = [
  {
    icon: Zap,
    title: 'Reusable prompts',
    description: 'Template library with variables and MCP exposure.',
  },
  {
    icon: Shield,
    title: 'Your data, your control',
    description: 'Local-first storage with optional sync.',
  },
  {
    icon: Users,
    title: 'Teams & agents',
    description: 'Share prompts and orchestrate A2A workflows.',
  },
] as const

function PitchPanel() {
  return (
    <div className="bg-muted relative hidden flex-col justify-between overflow-hidden border-r p-10 lg:flex lg:p-14">
      {/* Subtle dot grid backdrop */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 opacity-60 dark:opacity-50"
        style={{
          backgroundImage:
            'radial-gradient(circle at 1px 1px, var(--border) 1px, transparent 0)',
          backgroundSize: '28px 28px',
          maskImage:
            'radial-gradient(ellipse at top right, black, transparent 70%)',
          WebkitMaskImage:
            'radial-gradient(ellipse at top right, black, transparent 70%)',
        }}
      />

      <div className="relative z-10 max-w-md">
        <Brand />
        <h2 className="mt-6 text-3xl leading-tight font-bold tracking-tight">
          Your personal AI command center.
        </h2>
        <p className="text-muted-foreground mt-3 max-w-sm text-sm leading-relaxed">
          Centralize your prompts, memories, artifacts, agents and MCP
          integrations across Claude Code, Cursor and VS Code.
        </p>
        <div className="mt-8 flex flex-col gap-4">
          {PITCH_BULLETS.map(({ icon: Icon, title, description }) => (
            <div key={title} className="flex items-start gap-3">
              <div className="bg-card border-border text-foreground grid h-8 w-8 shrink-0 place-items-center rounded-lg border">
                <Icon className="h-3.5 w-3.5" />
              </div>
              <div className="text-sm">
                <div className="font-semibold">{title}</div>
                <div className="text-muted-foreground text-xs leading-relaxed">
                  {description}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="text-muted-foreground relative z-10 flex gap-4 text-xs">
        <span>
          © {CURRENT_YEAR} {SITE_LEGAL_NAME}
        </span>
        <a href={SITE_URL} className="hover:text-foreground transition-colors">
          {SITE_DOMAIN}
        </a>
        <a
          href={`mailto:${SUPPORT_EMAIL}`}
          className="hover:text-foreground transition-colors"
        >
          support
        </a>
      </div>
    </div>
  )
}

export function SignInPage() {
  // providers === null means "still loading the enabled-providers list".
  const [providers, setProviders] = useState<AuthProvider[] | null>(null)
  const [providersError, setProvidersError] = useState<string>('')
  // signingIn holds the canonical name of the provider whose redirect is in
  // flight, so we can show a per-button spinner and disable the others.
  const [signingIn, setSigningIn] = useState<string | null>(null)
  const [error, setError] = useState<string>('')

  const { login } = useAuth()
  const { trackAuth } = useAnalytics()
  const [searchParams] = useSearchParams()
  // Where to send the user after login (e.g. back to an OAuth consent page).
  // Validated to a same-origin path; defaults to "/" when absent/unsafe.
  const returnTo = sanitizeReturnTo(searchParams.get('return_to'))

  // Track signin page view when component mounts
  useEffect(() => {
    try {
      trackAuth({
        eventType: 'signin_page_view',
      })
    } catch (analyticsError) {
      console.error('Failed to track signin page view:', analyticsError)
    }
  }, [trackAuth])

  // Fetch the deployment's enabled login providers so the picker reflects the
  // actual configuration rather than a hardcoded list.
  useEffect(() => {
    let active = true
    authService
      .getProviders()
      .then(fetched => {
        if (active) setProviders(fetched)
      })
      .catch((err: unknown) => {
        if (!active) return
        setProviders([])
        setProvidersError(
          err instanceof Error
            ? err.message
            : 'Failed to load sign-in options. Please try again.'
        )
      })
    return () => {
      active = false
    }
  }, [])

  const handleSignIn = async (provider: AuthProvider) => {
    setError('')
    setSigningIn(provider.name)

    try {
      sessionStore.set(STORAGE_KEYS.LOGIN_METHOD, provider.display_name)
      // Persist across the IdP round-trip; honored when the user lands back.
      stashReturnTo(returnTo)
      await login(provider.name)
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
    <div className="bg-background text-foreground relative min-h-screen">
      <div className="grid min-h-screen lg:grid-cols-2">
        <PitchPanel />

        {/* Top-right theme toggle */}
        <div className="fixed top-4 right-4 z-20">
          <ThemeToggle />
        </div>

        <div className="relative grid place-items-center p-6 lg:p-8">
          {/* Light dot-grid backdrop on small screens (no pitch panel) */}
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 opacity-60 lg:hidden dark:opacity-50"
            style={{
              backgroundImage:
                'radial-gradient(circle at 1px 1px, var(--border) 1px, transparent 0)',
              backgroundSize: '28px 28px',
              maskImage:
                'radial-gradient(ellipse at center, black 0%, transparent 70%)',
              WebkitMaskImage:
                'radial-gradient(ellipse at center, black 0%, transparent 70%)',
            }}
          />

          <div className="bg-card border-border relative z-10 w-full max-w-sm rounded-2xl border p-8 shadow-sm">
            {/* Brand only on small screens (pitch panel hidden) */}
            <div className="mb-5 lg:hidden">
              <Brand />
            </div>

            <h1 className="text-2xl font-bold tracking-tight">
              Sign in to {SITE_NAME}
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm">
              Your personal AI command center. Pick up where you left off.
            </p>

            {(error || providersError) && (
              <Alert variant="destructive" className="mt-5">
                <AlertTitle>Sign in error</AlertTitle>
                <AlertDescription>{error || providersError}</AlertDescription>
              </Alert>
            )}

            <div className="mt-6 flex flex-col gap-2">
              {providers === null ? (
                <Button disabled className="w-full">
                  <span className="border-primary-foreground/40 border-r-primary-foreground inline-block h-4 w-4 animate-spin rounded-full border-2" />
                  <span>Loading sign-in options…</span>
                </Button>
              ) : providers.length === 0 ? (
                !providersError && (
                  <p className="text-muted-foreground text-sm">
                    No login providers are configured for this deployment.
                  </p>
                )
              ) : (
                providers.map(provider => (
                  <Button
                    key={provider.name}
                    onClick={() => {
                      void handleSignIn(provider)
                    }}
                    disabled={signingIn !== null}
                    className="w-full"
                  >
                    {signingIn === provider.name ? (
                      <>
                        <span className="border-primary-foreground/40 border-r-primary-foreground inline-block h-4 w-4 animate-spin rounded-full border-2" />
                        <span>Signing you in…</span>
                      </>
                    ) : (
                      <>
                        <ProviderIcon name={provider.name} />
                        <span>Continue with {provider.display_name}</span>
                      </>
                    )}
                  </Button>
                ))
              )}
            </div>

            {/* Divider */}
            <div className="text-muted-foreground my-5 flex items-center gap-3 text-xs tracking-wider uppercase">
              <div className="bg-border h-px flex-1" />
              <span>or</span>
              <div className="bg-border h-px flex-1" />
            </div>

            <DevLogin returnTo={returnTo} onError={setError} />

            <p className="text-muted-foreground mt-6 text-center text-xs leading-relaxed">
              By continuing, you agree to our{' '}
              <a
                href={TERMS_URL}
                target="_blank"
                rel="noopener noreferrer"
                className="text-foreground border-border hover:border-foreground border-b transition-colors"
              >
                Terms
              </a>{' '}
              and{' '}
              <a
                href={PRIVACY_URL}
                target="_blank"
                rel="noopener noreferrer"
                className="text-foreground border-border hover:border-foreground border-b transition-colors"
              >
                Privacy Policy
              </a>
              .
            </p>
          </div>
        </div>
      </div>

      <CookieConsentBanner />
    </div>
  )
}
