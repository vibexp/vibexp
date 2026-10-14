import { useState } from 'react'
import { Link } from 'react-router'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useAuth } from '@/contexts/useAuth'
import { messageOf } from '@/pages/admin/settings/auth/authSettingsForm'
import { FieldFrame } from '@/pages/admin/settings/InstanceSettingsFields'
import {
  fieldDescribedBy,
  type FieldErrors,
  serverFieldErrors,
} from '@/pages/admin/settings/instanceSettingsForm'
import { authService } from '@/services/authService'
import { authSettingsService } from '@/services/authSettingsService'
import { ApiError } from '@/types/errors'

type Field = 'email' | 'password' | 'repeat'

/** The fields a server validation error can name. */
const SERVER_FIELDS: readonly ('email' | 'password')[] = ['email', 'password']

/** Shown instead of the server's wording, which names a config key bare. */
const NOT_ROOT_ADMIN =
  'This address is not a root instance admin. Use one listed under auth.instance_admins in the server configuration.'

/** What the form shows once the password is stored but signing in failed. */
interface PartialOutcome {
  email: string
  message: string
}

/**
 * Enough to catch a slip; the server decides what an address is. No dot is
 * asked of the domain: `admin@localhost` is a valid root admin.
 */
const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+$/

function clientErrors(
  email: string,
  password: string,
  repeat: string
): FieldErrors<Field> {
  const errors: FieldErrors<Field> = {}
  if (email.trim() === '') errors.email = 'Enter the email address.'
  else if (!EMAIL_SHAPE.test(email.trim()))
    errors.email = 'Enter a complete email address, such as admin@example.com.'
  if (password === '') errors.password = 'Enter a password.'
  if (repeat !== password) errors.repeat = 'The two passwords do not match.'
  return errors
}

/**
 * The server's field errors for this form. A `not_root_admin` on the email
 * gets this page's wording; password policy messages are shown as sent.
 */
function serverErrors(err: unknown): {
  fields: FieldErrors<Field>
  other: string | null
} | null {
  const split = serverFieldErrors(err, SERVER_FIELDS)
  if (!split) return null
  const fields: FieldErrors<Field> = { ...split.fields }
  if (fields.email) fields.email = NOT_ROOT_ADMIN
  return {
    fields,
    other: split.other.length > 0 ? split.other.join(' ') : null,
  }
}

/**
 * Shown when this browser is also signed in to the app (#1473): the signed-in
 * session takes precedence over the setup session, and only a setup session
 * may set a root admin's password. Signing out here keeps the setup session
 * (it has its own cookie), so the page works again after a reload.
 */
function SignedInNote({
  email,
  onSignedOut,
}: Readonly<{ email: string | null; onSignedOut: () => void }>) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const signOut = async () => {
    setBusy(true)
    setError(null)
    try {
      await authService.logout()
      onSignedOut()
    } catch (err) {
      setError(messageOf(err, 'Signing out did not work'))
      setBusy(false)
    }
  }

  return (
    <Alert data-testid="setup-signed-in-note">
      <AlertTitle>This browser is signed in</AlertTitle>
      <AlertDescription className="space-y-2">
        <p>
          {email ? `You are signed in as ${email}. ` : 'You are signed in. '}
          Only the setup session from the setup URL can set a root admin&apos;s
          password, and your signed-in session is used instead. Sign out here
          and the page works again, or open the setup URL in a private window.
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={() => {
            void signOut()
          }}
        >
          {busy ? 'Signing out…' : 'Sign out here'}
        </Button>
        {error && (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        )}
      </AlertDescription>
    </Alert>
  )
}

/**
 * "Use email and password" on the first-run setup page (#1442): makes an
 * instance usable with no identity provider. It gives a root instance admin
 * (an address in the server's `auth.instance_admins`) a password and switches
 * email and password sign-in on in one call, then signs in with the same
 * credentials. That sign-in is what ends setup mode, so the page is left with
 * a full load: the session cookie has to be read from scratch.
 *
 * `navigate` is injectable so a test does not leave the page.
 */
export function SetupPasswordSection({
  navigate = url => {
    globalThis.location.assign(url)
  },
  reload = () => {
    globalThis.location.reload()
  },
}: Readonly<{ navigate?: (url: string) => void; reload?: () => void }>) {
  const { isAuthenticated, user } = useAuth()
  // The server refused the bootstrap because this browser is signed in.
  const [signedInRefusal, setSignedInRefusal] = useState(false)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [repeat, setRepeat] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const [busy, setBusy] = useState(false)
  const [fromServer, setFromServer] = useState<FieldErrors<Field>>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [partial, setPartial] = useState<PartialOutcome | null>(null)

  // Client checks show only once the form was submitted, so an untouched form
  // is not red; server errors show until the field they name is edited.
  const errors: FieldErrors<Field> = {
    ...fromServer,
    ...(submitted ? clientErrors(email, password, repeat) : {}),
  }

  const edit = (field: Field, setValue: (value: string) => void) => {
    return (value: string) => {
      setValue(value)
      setFromServer(current =>
        Object.fromEntries(
          Object.entries(current).filter(([key]) => key !== field)
        )
      )
    }
  }

  const submit = async () => {
    setSubmitted(true)
    setFormError(null)
    if (Object.keys(clientErrors(email, password, repeat)).length > 0) return

    const address = email.trim()
    try {
      setBusy(true)
      await authSettingsService.bootstrapAdminPassword({
        email: address,
        password,
      })
    } catch (err) {
      if (err instanceof ApiError && err.status === 403) {
        // Only a setup session may set a root admin's password, and a signed-in
        // session takes precedence over it in this browser (#1473).
        setSignedInRefusal(true)
        setBusy(false)
        return
      }
      const split = serverErrors(err)
      if (split) {
        setFromServer(split.fields)
        setFormError(split.other)
      } else {
        setFormError(messageOf(err, 'The admin account could not be set up'))
      }
      setBusy(false)
      return
    }

    // The password is stored and password sign-in is on from here.
    await signIn(address)
  }

  const signIn = async (address: string) => {
    try {
      setBusy(true)
      await authService.passwordLogin(address, password)
    } catch (err) {
      setPartial({
        email: address,
        message: messageOf(err, 'Signing in failed'),
      })
      setBusy(false)
      return
    }
    navigate('/')
  }

  if (partial) {
    return (
      <Card data-testid="setup-password-section">
        <CardHeader>
          <CardTitle>Use email and password</CardTitle>
        </CardHeader>
        <CardContent>
          <Alert data-testid="setup-password-partial">
            <AlertTitle>The admin account is ready</AlertTitle>
            <AlertDescription className="space-y-2">
              <p>
                The admin account for {partial.email} has the password you
                chose, and password sign-in is switched on. Only signing you in
                automatically did not work ({partial.message}).
              </p>
              <p>
                Try again here, or{' '}
                <Link
                  to="/login"
                  className="font-medium underline underline-offset-4"
                >
                  go to the sign-in page
                </Link>{' '}
                and sign in with that email and password. Signing in finishes
                setup.
              </p>
              <Button
                type="button"
                disabled={busy}
                data-testid="setup-password-retry"
                onClick={() => {
                  void signIn(partial.email)
                }}
              >
                {busy ? 'Signing in…' : 'Try signing in again'}
              </Button>
            </AlertDescription>
          </Alert>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card data-testid="setup-password-section">
      <CardHeader>
        <CardTitle>Use email and password</CardTitle>
        <CardDescription>
          No sign-in provider needed. Give a root instance admin (an address
          listed under auth.instance_admins in the server configuration) a
          password. The account is created if it does not exist; if it does, its
          password is replaced and it is signed out everywhere. This switches
          password sign-in on and signs you in, which finishes setup.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {((isAuthenticated && user?.is_instance_admin === true) ||
          signedInRefusal) && (
          <SignedInNote email={user?.email ?? null} onSignedOut={reload} />
        )}
        <form
          aria-label="Use email and password"
          className="max-w-md space-y-4"
          noValidate
          onSubmit={event => {
            event.preventDefault()
            void submit()
          }}
        >
          <FieldFrame
            id="setup-admin-email"
            label="Root admin email"
            error={errors.email}
          >
            <Input
              id="setup-admin-email"
              type="text"
              inputMode="email"
              autoComplete="username"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              placeholder="admin@example.com"
              value={email}
              disabled={busy}
              aria-invalid={errors.email ? true : undefined}
              aria-describedby={fieldDescribedBy(
                'setup-admin-email',
                false,
                errors.email
              )}
              onChange={event => {
                edit('email', setEmail)(event.target.value)
              }}
            />
          </FieldFrame>

          <FieldFrame
            id="setup-admin-password"
            label="Password"
            hint="At least 12 characters. Not a common password, and not the email address."
            error={errors.password}
          >
            <Input
              id="setup-admin-password"
              type="password"
              autoComplete="new-password"
              value={password}
              disabled={busy}
              aria-invalid={errors.password ? true : undefined}
              aria-describedby={fieldDescribedBy(
                'setup-admin-password',
                true,
                errors.password
              )}
              onChange={event => {
                edit('password', setPassword)(event.target.value)
              }}
            />
          </FieldFrame>

          <FieldFrame
            id="setup-admin-repeat"
            label="Repeat the password"
            error={errors.repeat}
          >
            <Input
              id="setup-admin-repeat"
              type="password"
              autoComplete="new-password"
              value={repeat}
              disabled={busy}
              aria-invalid={errors.repeat ? true : undefined}
              aria-describedby={fieldDescribedBy(
                'setup-admin-repeat',
                false,
                errors.repeat
              )}
              onChange={event => {
                setRepeat(event.target.value)
              }}
            />
          </FieldFrame>

          {formError && (
            <Alert variant="destructive" data-testid="setup-password-error">
              <AlertTitle>The admin account was not set up</AlertTitle>
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          )}

          <Button
            type="submit"
            disabled={busy}
            data-testid="setup-password-submit"
          >
            {busy ? 'Setting up…' : 'Set up admin and sign in'}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
