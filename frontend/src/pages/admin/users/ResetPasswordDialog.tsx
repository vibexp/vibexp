import { useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { formatDateTime } from '@/lib/time'
import {
  adminUserAccessService,
  type AdminUserResetLink,
} from '@/services/adminUserAccessService'
import { ApiError } from '@/types/errors'
import { getErrorMessage } from '@/utils/errorHandling'

import { type LinkDelivery, LinkDeliveryChoice } from './LinkDeliveryChoice'
import { LinkReveal } from './LinkReveal'
import { EMAIL_UNAVAILABLE_MESSAGE, isEmailUnavailable } from './setupOutcome'

type Delivery = LinkDelivery

function refusalMessage(err: unknown, invalidateNow: boolean): string {
  if (err instanceof ApiError) {
    if (isEmailUnavailable(err)) return EMAIL_UNAVAILABLE_MESSAGE
    if (err.code === 'email_unverified_shared') {
      // The server's sentence already says what to do instead.
      return err.message
    }
    if (err.status >= 500 && invalidateNow) {
      return 'The reset link could not be made. The current password may already be disabled: make a new link to be sure they can get back in.'
    }
  }
  return getErrorMessage(err, 'The reset link could not be made')
}

/**
 * "Reset password" on a user page (#1443, #1436 part B): a one-time link to
 * set a new password, emailed or shown here once. With "Sign out and disable
 * the current password now" the current password stops working, every
 * session ends and every earlier link dies before the new link is made; that
 * is asked twice. Without it the current password keeps working until the
 * link is used. The link lives in this dialog's state only.
 */
export function ResetPasswordDialog({
  open,
  onOpenChange,
  userId,
  email,
  hasPassword = true,
  emailLinks = null,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  email: string
  /**
   * False for an account that signs in only through identity providers: the
   * link sets a first password, and there is no current one to stop.
   */
  hasPassword?: boolean
  /** Whether a link can be emailed; false offers only "Show the link here". */
  emailLinks?: boolean | null
}>) {
  const [invalidateNow, setInvalidateNow] = useState(false)
  // A disabling reset waiting for the admin's second yes.
  const [confirming, setConfirming] = useState<Delivery | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<AdminUserResetLink | null>(null)

  const close = (next: boolean) => {
    if (!next) {
      setInvalidateNow(false)
      setConfirming(null)
      setError(null)
      setResult(null)
    }
    onOpenChange(next)
  }

  const issue = async (delivery: Delivery) => {
    setBusy(true)
    setError(null)
    setConfirming(null)
    try {
      setResult(
        await adminUserAccessService.issueResetLink(userId, {
          delivery,
          invalidate_now: invalidateNow,
        })
      )
    } catch (err) {
      setError(refusalMessage(err, invalidateNow))
    } finally {
      setBusy(false)
    }
  }

  const choose = (delivery: Delivery) => {
    if (invalidateNow) {
      setConfirming(delivery)
      return
    }
    void issue(delivery)
  }

  const until = result ? formatDateTime(result.expires_at) : ''

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent
        className="sm:max-w-lg"
        data-testid="reset-password-dialog"
      >
        <DialogHeader>
          <DialogTitle>
            {hasPassword ? 'Reset password' : 'Send a link to set a password'}
          </DialogTitle>
          <DialogDescription>
            A one-time link for {email} to set{' '}
            {hasPassword
              ? 'a new password'
              : 'a password, so they can sign in without their identity provider'}
            . An emailed link works for 1 hour, a link you hand over for 24
            hours.
          </DialogDescription>
        </DialogHeader>

        {result === null && (
          // min-w-0: a grid item, so a long address cannot widen the dialog.
          <div className="min-w-0 space-y-4">
            {hasPassword && (
              <div className="flex items-start gap-2">
                <Checkbox
                  id="reset-invalidate-now"
                  checked={invalidateNow}
                  disabled={busy}
                  onCheckedChange={value => {
                    setInvalidateNow(value === true)
                    setConfirming(null)
                  }}
                  className="mt-0.5"
                />
                <div className="space-y-1">
                  <Label htmlFor="reset-invalidate-now">
                    Sign out and disable the current password now
                  </Label>
                  <p className="text-muted-foreground text-sm">
                    Use it if the password may be known to someone else. Without
                    it, the current password and their sessions keep working
                    until they use the link.
                  </p>
                </div>
              </div>
            )}

            {confirming === null ? (
              <LinkDeliveryChoice
                email={email}
                emailLinks={emailLinks}
                busy={busy}
                onChoose={choose}
              />
            ) : (
              <Alert variant="destructive" data-testid="reset-confirm">
                <AlertTitle>Disable the current password now?</AlertTitle>
                <AlertDescription className="space-y-2">
                  <p>
                    {email} is signed out everywhere and cannot sign in with
                    their password until they use the new link. Any earlier link
                    stops working.
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="destructive"
                      size="sm"
                      disabled={busy}
                      onClick={() => {
                        void issue(confirming)
                      }}
                    >
                      Disable and{' '}
                      {confirming === 'email'
                        ? 'email the link'
                        : 'show the link'}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      onClick={() => {
                        setConfirming(null)
                      }}
                    >
                      Back
                    </Button>
                  </div>
                </AlertDescription>
              </Alert>
            )}

            {error && (
              <p role="alert" className="text-destructive text-sm">
                {error}
              </p>
            )}
          </div>
        )}

        {result && (
          <div className="space-y-3">
            {result.invalidated && (
              <p className="text-sm" data-testid="reset-invalidated">
                Their current password is disabled and they were signed out
                everywhere.
              </p>
            )}
            {result.delivery === 'email' && (
              <Alert data-testid="reset-link-emailed">
                <AlertTitle>Reset link emailed</AlertTitle>
                <AlertDescription>
                  Sent to {email}. It works until {until}.
                </AlertDescription>
              </Alert>
            )}
            {result.delivery === 'link' && result.link_url && (
              <div className="space-y-2">
                <p className="text-sm">It works until {until}.</p>
                <LinkReveal link={result.link_url} />
              </div>
            )}
          </div>
        )}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              close(false)
            }}
          >
            {result ? 'Done' : 'Cancel'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
