import { useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { formatDateTime } from '@/lib/time'
import {
  adminUserAccessService,
  type AdminUserSetupLink,
} from '@/services/adminUserAccessService'
import { getErrorMessage } from '@/utils/errorHandling'

import { type LinkDelivery, LinkDeliveryChoice } from './LinkDeliveryChoice'
import { LinkReveal } from './LinkReveal'
import { EMAIL_UNAVAILABLE_MESSAGE, isEmailUnavailable } from './setupOutcome'

/**
 * "Get setup link" on a user page (#1443): a new one-time link for an account
 * with no way to sign in yet, emailed or shown here once. Any earlier setup
 * link stops working, emailed or shown (#1490). The link lives in this
 * dialog's state and is dropped when it closes.
 */
export function SetupLinkDialog({
  open,
  onOpenChange,
  userId,
  email,
  emailLinks = null,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  email: string
  /** Whether a link can be emailed; false offers only "Show the link here". */
  emailLinks?: boolean | null
}>) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<AdminUserSetupLink | null>(null)

  const close = (next: boolean) => {
    if (!next) {
      setResult(null)
      setError(null)
    }
    onOpenChange(next)
  }

  const issue = async (delivery: LinkDelivery) => {
    setBusy(true)
    setError(null)
    try {
      setResult(
        await adminUserAccessService.issueSetupLink(userId, { delivery })
      )
    } catch (err) {
      // The 409s (`email_unavailable`, `has_sign_in_method`, …) carry a
      // sentence fit to show.
      setError(
        isEmailUnavailable(err)
          ? EMAIL_UNAVAILABLE_MESSAGE
          : getErrorMessage(err, 'The setup link could not be made')
      )
    } finally {
      setBusy(false)
    }
  }

  const until = result ? formatDateTime(result.expires_at) : ''

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-lg" data-testid="setup-link-dialog">
        <DialogHeader>
          <DialogTitle>Setup link</DialogTitle>
          <DialogDescription>
            A link for {email} to set their first password. It works once, for 7
            days, and any earlier setup link stops working.
          </DialogDescription>
        </DialogHeader>

        {result === null && (
          // min-w-0: a grid item, so a long address cannot widen the dialog.
          <div className="min-w-0 space-y-3">
            <LinkDeliveryChoice
              email={email}
              emailLinks={emailLinks}
              busy={busy}
              onChoose={delivery => {
                void issue(delivery)
              }}
            />
            {error && (
              <p role="alert" className="text-destructive text-sm">
                {error}
              </p>
            )}
          </div>
        )}

        {result?.delivery === 'email' && (
          <Alert data-testid="setup-link-emailed">
            <AlertTitle>Setup link emailed</AlertTitle>
            <AlertDescription>
              Sent to {email}. It works until {until}.
            </AlertDescription>
          </Alert>
        )}
        {result?.delivery === 'link' && result.link_url && (
          <div className="space-y-2">
            <p className="text-sm">It works until {until}.</p>
            <LinkReveal link={result.link_url} />
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
