import { ChevronRight, Code2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

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
  onError?: (error: string) => void
  /**
   * Same-origin path to land on after a successful dev login (e.g. an OAuth
   * consent page). Validated; defaults to "/".
   */
  returnTo?: string
}

export function DevLogin({ onError, returnTo }: Readonly<DevLoginProps>) {
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [open, setOpen] = useState(false)
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

  if (!shouldShow) {
    return null
  }

  const handleDevLogin = async (e: React.SubmitEvent) => {
    e.preventDefault()

    if (!email) {
      onError?.('Email is required')
      return
    }

    setIsLoading(true)

    try {
      await authService.devLogin(email, name || undefined)
      // Hard navigation (full reload) so the auth context re-hydrates from the
      // freshly-set session cookie, similar to the OAuth callback. Land on the
      // requested return path (e.g. back on the OAuth consent page) or "/".
      hardRedirect(sanitizeReturnTo(returnTo))
    } catch (err) {
      onError?.(mapSignInError(err, 'Dev login failed'))
      setIsLoading(false)
    }
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button
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
              id="dev-email"
              type="email"
              value={email}
              onChange={e => {
                setEmail(e.target.value)
              }}
              placeholder="dev@example.com"
              disabled={isLoading}
              className="h-9"
            />
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
