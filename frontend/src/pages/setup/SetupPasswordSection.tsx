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
import { FieldFrame } from '@/pages/admin/settings/InstanceSettingsFields'
import {
  fieldDescribedBy,
  type FieldErrors,
  serverFieldErrors,
} from '@/pages/admin/settings/instanceSettingsForm'
import { authService } from '@/services/authService'
import { authSettingsService } from '@/services/authSettingsService'

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

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback
}

function clientErrors(
  email: string,
  password: string,
  repeat: string
): FieldErrors<Field> {
  const errors: FieldErrors<Field> = {}
  if (email.trim() === '') errors.email = 'Enter the email address.'
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
}: Readonly<{ navigate?: (url: string) => void }>) {
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
      const split = serverErrors(err)
      if (split) {
        setFromServer(split.fields)
        setFormError(split.other)
      } else {
        setFormError(messageOf(err, 'The admin account could not be created'))
      }
      setBusy(false)
      return
    }

    // The password is stored and password sign-in is on from here.
    try {
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
                The admin account for {partial.email} was created with the
                password you chose, and password sign-in is switched on. Only
                signing you in automatically did not work ({partial.message}).
              </p>
              <p>
                <Link
                  to="/login"
                  className="font-medium underline underline-offset-4"
                >
                  Go to the sign-in page
                </Link>{' '}
                and sign in with that email and password to finish setup.
              </p>
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
          password. This switches password sign-in on and signs you in, which
          finishes setup.
        </CardDescription>
      </CardHeader>
      <CardContent>
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
              <AlertTitle>The admin account was not created</AlertTitle>
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          )}

          <Button
            type="submit"
            disabled={busy}
            data-testid="setup-password-submit"
          >
            {busy ? 'Creating…' : 'Create admin and sign in'}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
