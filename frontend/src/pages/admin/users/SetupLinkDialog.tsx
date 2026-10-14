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
import { adminService, type AdminUserSetupLink } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

import { LinkReveal } from './LinkReveal'

/**
 * "Get setup link" on a user page (#1443): a new one-time link for an account
 * with no way to sign in yet, emailed or shown here once. Any earlier link
 * stops working. The link lives in this dialog's state and is dropped when it
 * closes.
 */
export function SetupLinkDialog({
  open,
  onOpenChange,
  userId,
  email,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  email: string
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

  const issue = async (delivery: 'email' | 'link') => {
    setBusy(true)
    setError(null)
    try {
      setResult(await adminService.issueSetupLink(userId, { delivery }))
    } catch (err) {
      // The 409s (`email_unavailable`, `has_sign_in_method`, …) carry a
      // sentence fit to show.
      setError(getErrorMessage(err, 'The setup link could not be made'))
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
          <div className="space-y-3">
            <div className="flex flex-col gap-2 sm:flex-row">
              <Button
                type="button"
                disabled={busy}
                onClick={() => {
                  void issue('email')
                }}
              >
                Email it to {email}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => {
                  void issue('link')
                }}
              >
                Show the link here
              </Button>
            </div>
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
