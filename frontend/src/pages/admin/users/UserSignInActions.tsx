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
 * An account that signs in only through identity providers (#1443): an
 * administrator can send it a link to set a first password, the way back in
 * when the instance loses or switches off that provider.
 */
function providerOnly(user: AdminUserDetail): boolean {
  return (
    !user.pending_setup && !user.has_password && user.sign_in_methods.length > 0
  )
}

/**
 * Why the link actions are unavailable, as text under the page header: a
 * disabled button's `title` reaches neither keyboard nor screen reader users.
 */
export function signInActionsNote(
  user: AdminUserDetail,
  actionable: boolean,
  passwordLogin: boolean | null
): string | null {
  if (!actionable) return null
  const wantsLink =
    user.pending_setup || user.has_password || providerOnly(user)
  if (!wantsLink) return null
  if (user.status === 'suspended') {
    return 'Setup and reset links cannot be sent to a suspended account. Reactivate it first.'
  }
  if (passwordLogin === false) {
    return user.pending_setup
      ? `Setup links need password sign-in, which is switched off on this instance. This person can sign in through an identity provider with ${user.email}.`
      : 'Password links need password sign-in, which is switched off on this instance.'
  }
  return null
}

/**
 * A user page's sign-in actions and their dialogs (#1443): "Get setup link"
 * for an account with no way in yet, "Reset password" and "Remove password"
 * for one with a password, and "Send a link to set a password" for one that
 * signs in only through identity providers. Never for your own account (`actionable`). Links
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
  // A suspended account cannot get a link (409 `account_suspended`); its
  // password can still be removed.
  const linksAllowed = user.status !== 'suspended'

  return (
    <>
      {user.pending_setup && linksAllowed && (
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
      {providerOnly(user) && linksAllowed && (
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
          Send a link to set a password
        </Button>
      )}
      {user.has_password && (
        <>
          {linksAllowed && (
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
          )}
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
        hasPassword={user.has_password}
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
