import { ChevronRight, Code2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

import { authService } from '../../services/authService'
import { environmentService } from '../../services/environmentService'
import { mapSignInError } from '../../utils/authErrors'
import { hardRedirect } from '../../utils/navigation'
import { sanitizeReturnTo } from '../../utils/returnTo'

interface DevLoginProps {
  /**
   * Same-origin path to land on after a successful dev login (e.g. an OAuth
   * consent page). Validated; defaults to "/".
   */
  returnTo?: string
}

export function DevLogin({ returnTo }: Readonly<DevLoginProps>) {
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  // Shown under the email field, where the person is looking: the page-level
  // alert sits above the fold of this collapsed section and went unseen.
  const [error, setError] = useState('')
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const emailRef = useRef<HTMLInputElement>(null)
  // Set by a toggle the person made, so the section mounting does not take
  // focus on its own.
  const moveFocus = useRef(false)
  const [searchParams] = useSearchParams()
  const [shouldShow, setShouldShow] = useState(false)

  useEffect(() => {
    if (!environmentService.isDevLoginEnabled()) {
      setShouldShow(false)
      return
    }

    const devLoginParam = searchParams.get('dev_login')

    if (devLoginParam !== null) {
      setShouldShow(devLoginParam === 'true')
    } else {
      setShouldShow(true)
    }
  }, [searchParams])

  // Opening leads straight to the first field; closing hands focus back to the
  // toggle, since the field that held it is gone.
  useEffect(() => {
    if (!moveFocus.current) return
    moveFocus.current = false
    if (open) emailRef.current?.focus()
    else triggerRef.current?.focus()
  }, [open])

  if (!shouldShow) {
    return null
  }

  const handleDevLogin = async (e: React.SubmitEvent) => {
    e.preventDefault()

    if (!email) {
      setError('Email is required')
      return
    }

    setError('')
    setIsLoading(true)

    try {
      await authService.devLogin(email, name || undefined)
      // Hard navigation (full reload) so the auth context re-hydrates from the
      // freshly-set session cookie, similar to the OAuth callback. Land on the
      // requested return path (e.g. back on the OAuth consent page) or "/".
      hardRedirect(sanitizeReturnTo(returnTo))
    } catch (err) {
      setError(mapSignInError(err, 'Dev login failed'))
      setIsLoading(false)
    }
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={next => {
        moveFocus.current = true
        setOpen(next)
      }}
    >
      <CollapsibleTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          className="border-border text-muted-foreground hover:bg-muted hover:text-foreground flex w-full items-center justify-between rounded-md border border-dashed px-3 py-2 text-xs transition-colors"
        >
          <span className="flex items-center gap-2">
            <Code2 className="h-3.5 w-3.5" />
            <span>Development login</span>
          </span>
          <ChevronRight
            className={cn(
              'h-3.5 w-3.5 transition-transform duration-200',
              open && 'rotate-90'
            )}
          />
        </button>
      </CollapsibleTrigger>

      <CollapsibleContent className="overflow-hidden">
        <form
          onSubmit={e => void handleDevLogin(e)}
          action="javascript:void(0)"
          className="mt-3 space-y-3"
        >
          <div className="space-y-1.5">
            <Label htmlFor="dev-email" className="text-xs">
              Email
            </Label>
            <Input
              ref={emailRef}
              id="dev-email"
              type="email"
              value={email}
              onChange={e => {
                setEmail(e.target.value)
                setError('')
              }}
              // No address is special: any one signs in unless this instance
              // has an access allowlist, in which case only a listed one does.
              placeholder="you@example.com"
              disabled={isLoading}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? 'dev-email-error' : 'dev-email-hint'}
              className="h-9"
            />
            {error ? (
              <p
                id="dev-email-error"
                role="alert"
                className="text-destructive text-xs"
              >
                {error}
              </p>
            ) : (
              <p id="dev-email-hint" className="text-muted-foreground text-xs">
                Any address works, unless this instance limits sign-in to an
                allowlist.
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="dev-name" className="text-xs">
              Name{' '}
              <span className="text-muted-foreground font-normal">
                (optional)
              </span>
            </Label>
            <Input
              id="dev-name"
              type="text"
              value={name}
              onChange={e => {
                setName(e.target.value)
              }}
              placeholder="Dev User"
              disabled={isLoading}
              className="h-9"
            />
          </div>

          <Button
            type="submit"
            disabled={isLoading || !email}
            className="w-full"
          >
            {isLoading ? (
              <>
                <span className="border-primary-foreground/40 border-r-primary-foreground inline-block h-3.5 w-3.5 animate-spin rounded-full border-2" />
                <span>Signing you in…</span>
              </>
            ) : (
              <span>Dev login</span>
            )}
          </Button>

          <p className="text-muted-foreground text-xs">
            Development/testing only. Disabled in production.
          </p>
        </form>
      </CollapsibleContent>
    </Collapsible>
  )
}
