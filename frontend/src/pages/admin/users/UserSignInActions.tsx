import { KeyRound, KeySquare, Link2 } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import type { AdminUserDetail } from '@/services/adminService'

import { RemovePasswordDialog } from './RemovePasswordDialog'
import { ResetPasswordDialog } from './ResetPasswordDialog'
import { SetupLinkDialog } from './SetupLinkDialog'

const PASSWORD_OFF = 'Password sign-in is switched off on this instance.'

/**
 * A user page's sign-in actions and their dialogs (#1443): "Get setup link"
 * for an account with no way in yet, "Reset password" and "Remove password"
 * for one with a password. Never for your own account (`actionable`). Links
 * need password sign-in on the instance; removing a password does not.
 * `onChanged` re-reads the account after anything that may have changed it.
 */
export function UserSignInActions({
  user,
  actionable,
  passwordLogin,
  onChanged,
}: Readonly<{
  user: AdminUserDetail
  actionable: boolean
  passwordLogin: boolean | null
  onChanged: () => void
}>) {
  const [setupOpen, setSetupOpen] = useState(false)
  const [resetOpen, setResetOpen] = useState(false)
  const [removeOpen, setRemoveOpen] = useState(false)
  if (!actionable) return null

  const linksOff = passwordLogin === false

  return (
    <>
      {user.pending_setup && (
        <Button
          variant="outline"
          size="sm"
          disabled={linksOff}
          title={linksOff ? PASSWORD_OFF : undefined}
          onClick={() => {
            setSetupOpen(true)
          }}
        >
          <Link2 className="mr-2 size-4" aria-hidden />
          Get setup link
        </Button>
      )}
      {user.has_password && (
        <>
          <Button
            variant="outline"
            size="sm"
            disabled={linksOff}
            title={linksOff ? PASSWORD_OFF : undefined}
            onClick={() => {
              setResetOpen(true)
            }}
          >
            <KeyRound className="mr-2 size-4" aria-hidden />
            Reset password
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setRemoveOpen(true)
            }}
          >
            <KeySquare className="mr-2 size-4" aria-hidden />
            Remove password
          </Button>
        </>
      )}

      <SetupLinkDialog
        open={setupOpen}
        onOpenChange={open => {
          setSetupOpen(open)
          // The account may have been set up meanwhile.
          if (!open) onChanged()
        }}
        userId={user.id}
        email={user.email}
      />
      <ResetPasswordDialog
        open={resetOpen}
        onOpenChange={open => {
          setResetOpen(open)
          // A disabling reset changes the account's sign-in methods.
          if (!open) onChanged()
        }}
        userId={user.id}
        email={user.email}
      />
      <RemovePasswordDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        userId={user.id}
        email={user.email}
        onRemoved={() => {
          toast.success('Password removed')
          onChanged()
        }}
      />
    </>
  )
}
