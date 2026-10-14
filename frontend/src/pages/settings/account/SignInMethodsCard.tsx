import { useState } from 'react'
import { Link } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatDate } from '@/lib/time'
import { toast } from '@/lib/toast'
import {
  type Account,
  type AccountDisconnectedIdentity,
  type AccountIdentity,
  accountService,
} from '@/services/accountService'

import {
  accountErrorMessage,
  canDisconnect,
  canRemovePassword,
  hasGuardedAction,
  passwordDisabled,
  SIGN_IN_AGAIN_HREF,
} from './accountRules'
import { PasswordForm } from './PasswordForm'
import type { AccountState } from './useAccount'

/** What the person asked to remove, pending their confirmation. */
type Removal =
  { kind: 'password' } | { kind: 'identity'; identity: AccountIdentity }

const LAST_METHOD_HINT =
  'This is your only way to sign in, so it cannot be removed.'
// An administrator disabled the password (#1436 part B).
const PASSWORD_DISABLED_HINT =
  'It does not sign you in, and only an administrator can change it. Ask them for a link to set a new password.'
// Worded around what is missing: true also while password sign-in is off and
// the password is not a way in itself.
const PASSWORD_KEPT_HINT =
  'No enabled sign-in provider is connected to your account, so the password cannot be removed.'

function MethodRow({
  title,
  badge,
  detail,
  action,
  hint,
  testId,
}: Readonly<{
  title: string
  badge?: React.ReactNode
  detail: string
  action?: React.ReactNode
  hint?: string
  testId: string
}>) {
  return (
    <li
      className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between"
      data-testid={testId}
    >
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{title}</span>
          {badge}
        </div>
        <p className="text-muted-foreground text-sm">{detail}</p>
        {hint && (
          <p id={`${testId}-hint`} className="text-muted-foreground text-sm">
            {hint}
          </p>
        )}
      </div>
      {action}
    </li>
  )
}

function passwordDetail(account: Account): string {
  if (!account.password.set) return 'You have not set a password.'
  return account.password.changed_at
    ? `Last changed ${formatDate(account.password.changed_at)}.`
    : 'A password is set.'
}

function identityDetail(identity: AccountIdentity): string {
  const connected = `Connected ${formatDate(identity.connected_at)}.`
  const used = identity.last_used_at
    ? `Last used ${formatDate(identity.last_used_at)}.`
    : 'Not used to sign in yet.'
  return `${connected} ${used}`
}

/**
 * The provider accounts the owner disconnected (#1460). A sign-in through one
 * of them is refused until it is allowed again here. These are not sign-in
 * methods: nothing about removing a method reads this list.
 */
function DisconnectedList({
  state,
  account,
}: Readonly<{
  state: AccountState
  account: Account
}>) {
  // Shown beside the list, where the person clicked: the card's alert is
  // far above it on a long card.
  const [error, setError] = useState<string | null>(null)
  const entries = account.disconnected_identities
  if (entries.length === 0) return null

  const allowAgain = async (entry: AccountDisconnectedIdentity) => {
    setError(null)
    try {
      const sent = await state.change(() =>
        accountService.allowIdentityAgain(entry.id)
      )
      if (sent) {
        toast.success(
          `${entry.display_name} can sign in again. Your next sign-in through it connects it again if it uses this email address.`
        )
      }
    } catch (err) {
      setError(accountErrorMessage(err, 'It was not allowed again'))
    }
  }

  return (
    <section
      aria-labelledby="account-disconnected-heading"
      className="space-y-2"
      data-testid="account-disconnected"
    >
      <h3 id="account-disconnected-heading" className="font-medium">
        Disconnected
      </h3>
      <p className="text-muted-foreground text-sm">
        Signing in through these is refused until you allow it again.
      </p>
      {error && (
        <Alert variant="destructive" data-testid="account-disconnected-error">
          <AlertTitle>Nothing was changed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <ul className="divide-y">
        {entries.map(entry => (
          <MethodRow
            key={entry.id}
            testId={`account-disconnected-${entry.provider}`}
            title={entry.display_name}
            detail={`Disconnected ${formatDate(entry.disconnected_at)}.`}
            action={
              <Button
                variant="outline"
                size="sm"
                disabled={state.busy || state.needsSignIn}
                aria-label={`Allow ${entry.display_name} again`}
                onClick={() => {
                  void allowAgain(entry)
                }}
              >
                Allow again
              </Button>
            }
          />
        ))}
      </ul>
    </section>
  )
}

function removalCopy(removal: Removal): {
  title: string
  description: string
  confirm: string
  done: string
} {
  if (removal.kind === 'password') {
    return {
      title: 'Remove your password?',
      description:
        'You will sign in through your sign-in provider only. Your other devices are signed out.',
      confirm: 'Remove password',
      done: 'Password removed. Your other devices were signed out.',
    }
  }
  const name = removal.identity.display_name
  return {
    title: `Disconnect ${name}?`,
    description: `You will no longer sign in through ${name}, and your other devices are signed out. This ${name} account stays out until you allow it again here.`,
    confirm: `Disconnect ${name}`,
    done: `${name} disconnected. Your other devices were signed out.`,
  }
}

/**
 * How the account can be signed in to (#1441): the password and each identity
 * provider, with the password form underneath.
 */
export function SignInMethodsCard({
  state,
  account,
}: Readonly<{ state: AccountState; account: Account }>) {
  const [removal, setRemoval] = useState<Removal | null>(null)
  const [error, setError] = useState<string | null>(null)
  const { password } = account

  const confirmRemoval = async () => {
    if (!removal) return
    const copy = removalCopy(removal)
    setError(null)
    try {
      const sent = await state.change(() =>
        removal.kind === 'password'
          ? accountService.removePassword()
          : accountService.disconnectIdentity(removal.identity.id)
      )
      if (sent) toast.success(copy.done)
    } catch (err) {
      setError(accountErrorMessage(err, 'The sign-in method was not removed'))
    } finally {
      setRemoval(null)
    }
  }

  const removePasswordAllowed = canRemovePassword(account)
  const disabled = passwordDisabled(account)
  // Removing a method and setting a first password need a recent sign-in;
  // changing a password needs the current one instead.
  const guarded = state.needsSignIn
  // The note only when signing in again would unlock something, or the server
  // has just asked for it.
  const showSignInNote =
    state.reauthRefused || (guarded && hasGuardedAction(account))
  const copy = removal ? removalCopy(removal) : null

  return (
    <Card data-testid="account-sign-in-card">
      <CardHeader>
        <CardTitle>Sign-in methods</CardTitle>
        <CardDescription>
          The ways you can sign in to this account.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {showSignInNote && (
          <Alert data-testid="account-reauth-note">
            <AlertTitle>
              Sign in again to change your sign-in methods
            </AlertTitle>
            <AlertDescription className="space-y-2">
              <p>
                Setting a first password, removing your password, disconnecting
                a provider and allowing a disconnected one again are only
                possible within 10 minutes of signing in.
              </p>
              <Button asChild variant="outline" size="sm">
                <Link to={SIGN_IN_AGAIN_HREF}>Sign in again</Link>
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {error && (
          <Alert variant="destructive" data-testid="account-method-error">
            <AlertTitle>Nothing was changed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <ul className="divide-y">
          <MethodRow
            testId="account-password-row"
            title="Password"
            badge={
              disabled ? (
                <Badge variant="destructive">
                  Disabled by an administrator
                </Badge>
              ) : (
                <Badge variant={password.set ? 'secondary' : 'outline'}>
                  {password.set ? 'Set' : 'Not set'}
                </Badge>
              )
            }
            detail={passwordDetail(account)}
            hint={
              disabled
                ? PASSWORD_DISABLED_HINT
                : password.set && !removePasswordAllowed
                  ? PASSWORD_KEPT_HINT
                  : undefined
            }
            action={
              password.set && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={state.busy || guarded || !removePasswordAllowed}
                  aria-describedby={
                    removePasswordAllowed
                      ? undefined
                      : 'account-password-row-hint'
                  }
                  onClick={() => {
                    setRemoval({ kind: 'password' })
                  }}
                >
                  Remove password
                </Button>
              )
            }
          />
          {account.identities.map(identity => {
            const allowed = canDisconnect(account, identity)
            return (
              <MethodRow
                key={identity.id}
                testId={`account-identity-${identity.provider}`}
                title={identity.display_name}
                badge={
                  !identity.available && (
                    <Badge variant="outline">Provider disabled</Badge>
                  )
                }
                detail={identityDetail(identity)}
                hint={allowed ? undefined : LAST_METHOD_HINT}
                action={
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={state.busy || guarded || !allowed}
                    aria-label={`Disconnect ${identity.display_name}`}
                    aria-describedby={
                      allowed
                        ? undefined
                        : `account-identity-${identity.provider}-hint`
                    }
                    onClick={() => {
                      setRemoval({ kind: 'identity', identity })
                    }}
                  >
                    Disconnect
                  </Button>
                }
              />
            )
          })}
        </ul>

        {/* Keyed on the entries: a changed list starts with no stale error. */}
        <DisconnectedList
          key={account.disconnected_identities.map(d => d.id).join(',')}
          state={state}
          account={account}
        />

        <section
          aria-labelledby="account-password-heading"
          className="space-y-4"
        >
          <h3 id="account-password-heading" className="font-medium">
            {password.set ? 'Change password' : 'Set a password'}
          </h3>
          {disabled ? (
            <p
              className="text-muted-foreground text-sm"
              data-testid="account-password-admin-disabled"
            >
              An administrator disabled your password, so it cannot be changed
              here. Ask them for a link to set a new one; using it lifts the
              block.
            </p>
          ) : password.login_enabled ? (
            <PasswordForm
              state={state}
              hasPassword={password.set}
              blocked={!password.set && guarded}
            />
          ) : (
            <p
              className="text-muted-foreground text-sm"
              data-testid="account-password-disabled"
            >
              Your administrator has not switched password sign-in on for this
              instance, so a password cannot be set or changed here.
              {password.set &&
                ' The password you have is kept, but it cannot be used to sign in until password sign-in is switched on.'}
            </p>
          )}
        </section>
      </CardContent>

      {copy && (
        <ConfirmDialog
          open
          onOpenChange={open => {
            if (!open) setRemoval(null)
          }}
          title={copy.title}
          description={copy.description}
          confirmLabel={copy.confirm}
          variant="destructive"
          loading={state.busy}
          onConfirm={confirmRemoval}
        />
      )}
    </Card>
  )
}
