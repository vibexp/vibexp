import { zodResolver } from '@hookform/resolvers/zod'
import { KeyRound } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { useAccountLinkToken } from '@/features/password/accountLinkToken'
import { NewPasswordFields } from '@/features/password/NewPasswordFields'
import {
  newPasswordFields,
  passwordsMatch,
} from '@/features/password/newPasswordSchema'
import { PublicCardPage } from '@/features/site-info/PublicPageBrand'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'
import {
  type AccountLinkPreview,
  accountLinkService,
} from '@/services/accountLinkService'
import type { AuthProvider } from '@/services/authService'
import { ApiError } from '@/types/errors'

import { previewFailure, statusOf } from './accountLinkFailure'
import { CheckingLink, LinkNotice, SetButNotSignedIn } from './accountLinkParts'
import { ProviderChoices } from './ProviderChoices'
import { GoToSignIn, InstanceName } from './PublicCardParts'
import { enabledProviders, messageOf, optionalNameField } from './publicForms'

const schema = z
  .object({ name: optionalNameField, ...newPasswordFields })
  .superRefine(passwordsMatch)

type SetupValues = z.infer<typeof schema>

type View =
  | { kind: 'loading' }
  /** No token, an unusable one, or a reset link opened here. */
  | { kind: 'invalid' }
  /** Password sign-in is off on the instance. */
  | { kind: 'password-off'; providers: AuthProvider[] }
  /** A passing failure: the link may still work. */
  | { kind: 'refused'; message: string }
  /** The allowlist does not admit the account's email: retrying cannot help. */
  | { kind: 'restricted'; message: string }
  | { kind: 'ready'; preview: AccountLinkPreview; providers: AuthProvider[] }
  /** The password was set but the request did not finish. */
  | { kind: 'set-not-signed-in'; email: string }

/**
 * `/account/setup#token=…` (#1440): where an account an administrator created
 * gets its first password. The token is taken from the URL fragment and the
 * fragment removed before any request; it is only ever sent in a POST body.
 *
 * `go` performs a full page load (injectable for tests): after the sign-in the
 * app must start again to read the new session.
 */
export function AccountSetupPage({
  go = url => {
    globalThis.location.assign(url)
  },
}: Readonly<{ go?: (url: string) => void }>) {
  const token = useAccountLinkToken()
  const [view, setView] = useState<View>({ kind: 'loading' })

  const load = useCallback(async () => {
    if (!token) {
      setView({ kind: 'invalid' })
      return
    }
    setView({ kind: 'loading' })
    try {
      const [preview, providers] = await Promise.all([
        accountLinkService.preview(token),
        enabledProviders(),
      ])
      setView(
        preview.purpose === 'setup'
          ? { kind: 'ready', preview, providers }
          : { kind: 'invalid' }
      )
    } catch (err) {
      const failure = previewFailure(err)
      setView(
        failure.kind === 'password-off'
          ? { kind: 'password-off', providers: await enabledProviders() }
          : failure
      )
    }
  }, [token])

  useEffect(() => {
    void load()
  }, [load])

  if (view.kind === 'loading') return <CheckingLink />

  if (view.kind === 'ready') {
    return (
      <SetupForm
        token={token ?? ''}
        preview={view.preview}
        providers={view.providers}
        go={go}
        onInvalid={() => {
          setView({ kind: 'invalid' })
        }}
        onSetNotSignedIn={email => {
          setView({ kind: 'set-not-signed-in', email })
        }}
      />
    )
  }

  return (
    <PublicCardPage>
      <Card
        className="w-full max-w-md"
        data-testid={`account-setup-${view.kind}`}
      >
        <CardContent className="flex flex-col gap-4 p-6">
          <h1 className="sr-only">Set up your account</h1>
          <InstanceName />
          {view.kind === 'invalid' && (
            <>
              <LinkNotice
                title="This link is not valid"
                text="It may have expired or been used already. Ask your administrator for a new one. If you already set a password, sign in with it."
              />
              <GoToSignIn />
            </>
          )}
          {view.kind === 'restricted' && (
            <>
              <LinkNotice
                title="This account cannot be used here"
                text={view.message}
              />
              <GoToSignIn />
            </>
          )}
          {view.kind === 'refused' && (
            <>
              <LinkNotice
                title="The link could not be checked"
                text={view.message}
              />
              <Button
                variant="outline"
                onClick={() => {
                  void load()
                }}
              >
                Try again
              </Button>
            </>
          )}
          {view.kind === 'password-off' && (
            <>
              <LinkNotice
                title="Passwords are not used here"
                text="This instance does not offer password sign-in at the moment. Sign in with one of the providers below, using the email address your account was created for."
              />
              <ProviderChoices providers={view.providers} />
              <GoToSignIn />
            </>
          )}
          {view.kind === 'set-not-signed-in' && (
            <SetButNotSignedIn
              title="Your password is set"
              email={view.email}
            />
          )}
        </CardContent>
      </Card>
    </PublicCardPage>
  )
}

function SetupForm({
  token,
  preview,
  providers,
  go,
  onInvalid,
  onSetNotSignedIn,
}: Readonly<{
  token: string
  preview: AccountLinkPreview
  providers: AuthProvider[]
  go: (url: string) => void
  onInvalid: () => void
  onSetNotSignedIn: (email: string) => void
}>) {
  const { displayName } = useSiteInfo()
  const form = useForm<SetupValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: preview.name, password: '', confirmPassword: '' },
  })
  const [busy, setBusy] = useState(false)

  const submit = async (values: SetupValues) => {
    setBusy(true)
    try {
      await accountLinkService.completeSetup({
        token,
        password: values.password,
        ...(values.name ? { name: values.name } : {}),
      })
      go('/')
      return
    } catch (err) {
      setBusy(false)
      const status = statusOf(err)
      if (status === 401) {
        onInvalid()
        return
      }
      if (status === 500) {
        // The link is spent and the password was set: trying again cannot
        // succeed.
        onSetNotSignedIn(preview.email)
        return
      }
      const onPassword =
        err instanceof ApiError
          ? err.getFieldErrors('password').at(0)
          : undefined
      if (onPassword) {
        form.setError('password', { message: onPassword.message })
        return
      }
      // 503 stored nothing and 403/429 changed nothing: the link still
      // works, and the form keeps what was typed for another try.
      form.setError('root', {
        message:
          status === 503
            ? 'Nothing was saved: the server could not finish the request. Try again.'
            : messageOf(err, 'Your password could not be set'),
      })
    }
  }

  const rootError = form.formState.errors.root?.message

  return (
    <PublicCardPage>
      <Card className="w-full max-w-md" data-testid="account-setup-ready">
        <CardHeader className="text-center">
          <div className="bg-primary text-primary-foreground mx-auto mb-2 flex size-12 items-center justify-center rounded-full">
            <KeyRound className="size-6" />
          </div>
          <CardTitle as="h1">Set up your account</CardTitle>
          <CardDescription>
            Choose a password to sign in to {displayName}.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Form {...form}>
            <form
              aria-label="Set up your account"
              className="space-y-4"
              noValidate
              onSubmit={event => {
                void form.handleSubmit(submit)(event)
              }}
            >
              <div className="space-y-2">
                <Label htmlFor="account-setup-email">Email</Label>
                <Input
                  id="account-setup-email"
                  value={preview.email}
                  readOnly
                  disabled
                  autoComplete="username"
                />
              </div>
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Name (optional)</FormLabel>
                    <FormControl>
                      <Input {...field} autoComplete="name" disabled={busy} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <NewPasswordFields
                control={form.control}
                name="password"
                confirmName="confirmPassword"
                disabled={busy}
              />
              {rootError && (
                <p
                  role="alert"
                  className="text-destructive text-sm"
                  data-testid="account-setup-error"
                >
                  {rootError}
                </p>
              )}
              <Button type="submit" className="w-full" disabled={busy}>
                {busy ? 'Setting up…' : 'Create password and sign in'}
              </Button>
            </form>
          </Form>
          {providers.length > 0 && (
            <>
              <div className="text-muted-foreground flex items-center gap-3 text-xs">
                <Separator className="flex-1" />
                or sign in with a provider using {preview.email}
                <Separator className="flex-1" />
              </div>
              <ProviderChoices providers={providers} disabled={busy} />
            </>
          )}
        </CardContent>
      </Card>
    </PublicCardPage>
  )
}
