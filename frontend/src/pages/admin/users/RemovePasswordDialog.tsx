import { useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { adminUserAccessService } from '@/services/adminUserAccessService'
import { ApiError } from '@/types/errors'
import { getErrorMessage } from '@/utils/errorHandling'

/**
 * "Remove password" on a user page (#1443, #1436 part B), disabled or not.
 * Their signed-in sessions keep running. When the password is the account's
 * only way in, the server refuses with `last_sign_in_method`; the dialog then
 * asks again, saying so, and sends `confirm_last_method=true`.
 */
export function RemovePasswordDialog({
  open,
  onOpenChange,
  userId,
  email,
  onRemoved,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  email: string
  onRemoved: () => void
}>) {
  const [busy, setBusy] = useState(false)
  const [lastMethod, setLastMethod] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const close = (next: boolean) => {
    if (!next) {
      setLastMethod(false)
      setError(null)
    }
    onOpenChange(next)
  }

  const remove = async () => {
    setBusy(true)
    setError(null)
    try {
      await adminUserAccessService.removePassword(userId, lastMethod)
      close(false)
      onRemoved()
    } catch (err) {
      if (
        !lastMethod &&
        err instanceof ApiError &&
        err.code === 'last_sign_in_method'
      ) {
        setLastMethod(true)
        return
      }
      setError(getErrorMessage(err, 'The password could not be removed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={close}
      title={
        lastMethod
          ? 'This is their only way to sign in'
          : `Remove ${email}'s password?`
      }
      description={
        <>
          {lastMethod
            ? `Without the password ${email} cannot sign in at all until you send a setup link or they sign in through an identity provider with this address.`
            : `They can no longer sign in with a password. Their signed-in sessions keep running.`}
          {error && (
            <span role="alert" className="text-destructive mt-2 block">
              {error}
            </span>
          )}
        </>
      }
      confirmLabel={lastMethod ? 'Remove it anyway' : 'Remove password'}
      variant="destructive"
      loading={busy}
      onConfirm={remove}
    />
  )
}
