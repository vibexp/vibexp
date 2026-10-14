import { RefreshCw } from 'lucide-react'
import { Link } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
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
import type { AdminAuthRegistrationMode } from '@/services/authSettingsService'

import { SwitchSettingField } from '../InstanceSettingsFields'
import { lockoutRisksCopy } from './authSettingsForm'
import { useAuthPolicy } from './useAuthPolicy'

const REGISTRATION_MODES: readonly {
  value: AdminAuthRegistrationMode
  label: string
  description: string
}[] = [
  {
    value: 'open',
    label: 'Open',
    description:
      'Anyone a sign-in provider accepts gets an account the first time they sign in, within the access allowlist.',
  },
  {
    value: 'invite_only',
    label: 'Invitation only',
    description:
      'A new person gets an account only through a team invitation sent to their email, or when an administrator creates it. Accounts that already exist are not affected.',
  },
]

function RegistrationModeField({
  value,
  disabled,
  onChange,
}: Readonly<{
  value: AdminAuthRegistrationMode
  disabled: boolean
  onChange: (value: AdminAuthRegistrationMode) => void
}>) {
  return (
    <fieldset className="space-y-2" data-testid="policy-registration">
      <legend className="text-sm leading-none font-medium">Registration</legend>
      <p className="text-muted-foreground text-xs">
        Who can get a new account. An account with password sign-in is always by
        invitation, whatever is chosen here.
      </p>
      <div className="space-y-2">
        {REGISTRATION_MODES.map(mode => {
          const id = `registration-mode-${mode.value}`
          return (
            <div
              key={mode.value}
              className="flex items-start gap-3 rounded-md border p-3 text-sm"
            >
              <input
                type="radio"
                id={id}
                name="registration-mode"
                className="mt-1"
                value={mode.value}
                checked={value === mode.value}
                disabled={disabled}
                aria-describedby={`${id}-hint`}
                onChange={() => {
                  onChange(mode.value)
                }}
              />
              <div className="space-y-1">
                <label htmlFor={id} className="block font-medium">
                  {mode.label}
                </label>
                <p id={`${id}-hint`} className="text-muted-foreground text-xs">
                  {mode.description}
                </p>
              </div>
            </div>
          )
        })}
      </div>
    </fieldset>
  )
}

/**
 * The reasons of a lockout risk in the confirmation: one reason is its
 * explanation alone; several are each named with their explanation. Spans,
 * because the dialog's description is a paragraph.
 */
function LockoutReasons({
  items,
}: Readonly<{ items: { title: string; description: string }[] }>) {
  if (items.length === 1) return <>{items[0].description}</>
  return (
    <span className="block space-y-3" data-testid="lockout-reasons">
      {items.map((item, index) => (
        // The titles can repeat (two unknown reasons share the generic copy).

        <span key={index} className="block">
          <span className="text-foreground block font-medium">
            {item.title}
          </span>
          <span className="block">{item.description}</span>
        </span>
      ))}
    </span>
  )
}

/**
 * Sign-in policy (#1442): whether password sign-in is offered, and
 * who may get an account. The rules of a save — one version for both
 * settings, never retry a conflict, confirm a lockout risk — are
 * `useAuthPolicy`'s.
 */
export function AuthPolicySection({
  onSaved,
}: Readonly<{ onSaved?: () => void }>) {
  const policy = useAuthPolicy(onSaved)
  const { stored, form, loading, loadError, busy, conflict, dirty } = policy

  if (loading && !stored) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
    )
  }

  if (!stored) {
    return (
      <Alert variant="destructive" data-testid="policy-load-failed">
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>
          {loadError ?? 'Failed to load the sign-in policy'}
        </AlertDescription>
      </Alert>
    )
  }

  const disabled = busy || loading
  const lockout = policy.pendingLockout
    ? lockoutRisksCopy(
        policy.pendingLockout.reasons,
        policy.pendingLockout.accounts
      )
    : null

  return (
    <Card data-testid="auth-policy-section">
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>Sign-in and registration</CardTitle>
          <Badge
            variant={stored.password_login_enabled ? 'default' : 'secondary'}
            data-testid="policy-password-status"
          >
            {stored.password_login_enabled
              ? 'Password sign-in on'
              : 'Password sign-in off'}
          </Badge>
        </div>
        <CardDescription>
          Whether password sign-in is offered, and who can get a new account.
          Changes apply within a few seconds, with no restart.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          aria-label="Sign-in and registration"
          className="space-y-5"
          onSubmit={event => {
            event.preventDefault()
            void policy.save()
          }}
        >
          {loadError && (
            <Alert variant="destructive" data-testid="policy-load-error">
              <AlertTitle>Could not refresh the sign-in policy</AlertTitle>
              <AlertDescription>
                {loadError} The values below may be out of date.
              </AlertDescription>
            </Alert>
          )}

          {conflict && (
            <Alert variant="destructive" data-testid="policy-conflict">
              <AlertTitle>Someone else changed the sign-in policy</AlertTitle>
              <AlertDescription className="space-y-2">
                <p>
                  Your changes were not saved, so nothing was overwritten.
                  Reload to see the current settings; your unsaved choices will
                  be discarded.
                </p>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={loading}
                  onClick={() => {
                    void policy.reload()
                  }}
                >
                  <RefreshCw className="mr-2 size-4" />
                  Reload
                </Button>
              </AlertDescription>
            </Alert>
          )}

          <SwitchSettingField
            id="policy-password-login"
            label="Password sign-in"
            hint="People sign in with their email address and a password. A password is set through a team invitation or by an administrator; nobody can sign up on their own."
            checked={form.passwordLogin}
            disabled={disabled}
            onChange={policy.setPasswordLogin}
          />

          {stored.password_login_enabled && policy.resetByAdmin && (
            <Alert data-testid="policy-no-email-note">
              <AlertTitle>Forgotten passwords need an administrator</AlertTitle>
              <AlertDescription>
                No email provider is configured, so nobody can be sent a reset
                link. A person who forgets their password needs a reset link
                issued by an administrator.{' '}
                <Link
                  to="/admin/settings/email"
                  className="font-medium underline underline-offset-4"
                >
                  Set up email
                </Link>{' '}
                to let people reset their own.
              </AlertDescription>
            </Alert>
          )}

          <RegistrationModeField
            value={form.registrationMode}
            disabled={disabled}
            onChange={policy.setRegistrationMode}
          />

          {policy.formError && (
            <Alert variant="destructive" data-testid="policy-error">
              <AlertTitle>Not saved</AlertTitle>
              <AlertDescription>{policy.formError}</AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-muted-foreground text-sm">
              {dirty ? 'You have unsaved changes.' : ' '}
            </p>
            <Button
              type="submit"
              size="sm"
              disabled={disabled || !dirty || conflict}
            >
              {busy ? 'Saving…' : 'Save sign-in policy'}
            </Button>
          </div>
        </form>
      </CardContent>

      <ConfirmDialog
        open={lockout !== null}
        onOpenChange={open => {
          if (!open) policy.cancelLockout()
        }}
        title={lockout?.title ?? ''}
        description={lockout && <LockoutReasons items={lockout.items} />}
        confirmLabel="Switch it off anyway"
        variant="destructive"
        loading={busy}
        onConfirm={policy.confirmLockout}
      />
    </Card>
  )
}
