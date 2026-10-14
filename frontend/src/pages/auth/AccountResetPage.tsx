import { zodResolver } from '@hookform/resolvers/zod'
import { AlertCircle, KeyRound, Loader2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
import { ApiError } from '@/types/errors'

import { GoToSignIn, InstanceName } from './PublicCardParts'
import { messageOf } from './publicForms'

const schema = z
  .object({ ...newPasswordFields, revokeApiAccess: z.boolean() })
  .superRefine(passwordsMatch)

type ResetValues = z.infer<typeof schema>

type View =
  | { kind: 'loading' }
  /** No token, an unusable one (expired, used, unknown), or a setup link. */
  | { kind: 'invalid' }
  /** Password sign-in is off on the instance. */
  | { kind: 'password-off' }
  /** A passing failure: the link may still work. */
  | { kind: 'refused'; message: string }
  /** The allowlist does not admit the account's email. */
  | { kind: 'restricted'; message: string }
  | { kind: 'ready'; preview: AccountLinkPreview }
  /** The password was changed but the request did not finish. */
  | { kind: 'set-not-signed-in'; email: string }

function statusOf(err: unknown): number | null {
  return err instanceof ApiError ? err.status : null
}

/**
 * `/account/reset#token=…` (#1439): a reset link, emailed after "Forgot
 * password?" or handed over by an administrator. The token is taken from the
 * URL fragment and the fragment removed before any request; it is only ever
 * sent in a POST body.
 *
 * `go` performs a full page load (injectable for tests): completing the link
 * signs the person in, and the app must start again to read the new session.
 */
export function AccountResetPage({
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
      const preview = await accountLinkService.preview(token)
      setView(
        preview.purpose === 'reset'
          ? { kind: 'ready', preview }
          : { kind: 'invalid' }
      )
    } catch (err) {
      const status = statusOf(err)
      // 400: a token that cannot be one (oversized, malformed).
      if (status === 401 || status === 400) setView({ kind: 'invalid' })
      else if (status === 403)
        setView({
          kind: 'restricted',
          message: messageOf(err, 'Access to this instance is restricted.'),
        })
      else if (status === 404) setView({ kind: 'password-off' })
      else
        setView({
          kind: 'refused',
          message: messageOf(err, 'The link could not be checked'),
        })
    }
  }, [token])

  useEffect(() => {
    void load()
  }, [load])

  if (view.kind === 'loading') {
    return (
      <PublicCardPage>
        <div className="flex flex-col items-center gap-3 text-center">
          <Loader2 className="text-primary size-10 animate-spin" />
          <p className="text-muted-foreground text-sm">Checking your link…</p>
        </div>
      </PublicCardPage>
    )
  }

  if (view.kind === 'ready') {
    return (
      <ResetForm
        token={token ?? ''}
        preview={view.preview}
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
        data-testid={`account-reset-${view.kind}`}
      >
        <CardContent className="flex flex-col gap-4 p-6">
          <InstanceName />
          <h1 className="sr-only">Set a new password</h1>
          {view.kind === 'invalid' && (
            <>
              <Notice
                title="This link is not valid"
                text="It may have expired or been used already. Ask for a new one with “Forgot password?” on the sign-in page, or ask your administrator."
              />
              <GoToSignIn />
            </>
          )}
          {view.kind === 'restricted' && (
            <Notice
              title="This account cannot be used here"
              text={view.message}
            />
          )}
          {view.kind === 'refused' && (
            <>
              <Notice
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
              <Notice
                title="Passwords are not used here"
                text="This instance does not offer password sign-in at the moment. Sign in with one of the options on the sign-in page."
              />
              <GoToSignIn />
            </>
          )}
          {view.kind === 'set-not-signed-in' && (
            <>
              <Alert>
                <AlertTitle>Your password is changed</AlertTitle>
                <AlertDescription>
                  Signing you in did not finish. Sign in with {view.email} and
                  the password you just chose.
                </AlertDescription>
              </Alert>
              <GoToSignIn variant="default" />
            </>
          )}
        </CardContent>
      </Card>
    </PublicCardPage>
  )
}

function Notice({ title, text }: Readonly<{ title: string; text: string }>) {
  return (
    <Alert variant="destructive">
      <AlertCircle className="size-4" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{text}</AlertDescription>
    </Alert>
  )
}

function ResetForm({
  token,
  preview,
  go,
  onInvalid,
  onSetNotSignedIn,
}: Readonly<{
  token: string
  preview: AccountLinkPreview
  go: (url: string) => void
  onInvalid: () => void
  onSetNotSignedIn: (email: string) => void
}>) {
  const { displayName } = useSiteInfo()
  const form = useForm<ResetValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      password: '',
      confirmPassword: '',
      revokeApiAccess: false,
    },
  })
  const [busy, setBusy] = useState(false)

  const submit = async (values: ResetValues) => {
    setBusy(true)
    // Nothing else is in flight here: the response re-issues the session
    // cookie and ends every other session (#1428), so the next request is the
    // full page load below.
    try {
      await accountLinkService.completeReset({
        token,
        password: values.password,
        revoke_api_access: values.revokeApiAccess,
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
        // The link is spent and the password was changed: trying again
        // cannot succeed.
        onSetNotSignedIn(preview.email)
        return
      }
      const onPassword =
        err instanceof ApiError
          ? err.getFieldErrors('password').at(0)
          : undefined
      if (onPassword) {
        form.setError('password', { message: onPassword.message })
        form.setFocus('password')
        return
      }
      // 503 stored nothing and 403/429 changed nothing: the link still
      // works, and the form keeps what was typed for another try.
      form.setError('root', {
        message:
          status === 503
            ? 'Nothing was saved: the server could not finish the request. Try again.'
            : messageOf(err, 'Your password could not be changed'),
      })
    }
  }

  const rootError = form.formState.errors.root?.message

  return (
    <PublicCardPage>
      <Card className="w-full max-w-md" data-testid="account-reset-ready">
        <CardHeader className="text-center">
          <InstanceName />
          <div className="bg-primary text-primary-foreground mx-auto mb-2 flex size-12 items-center justify-center rounded-full">
            <KeyRound className="size-6" />
          </div>
          <CardTitle as="h1">Set a new password</CardTitle>
          <CardDescription>
            Choose a new password to sign in to {displayName}. Every other
            signed-in browser is signed out.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Form {...form}>
            <form
              aria-label="Set a new password"
              className="space-y-4"
              noValidate
              onSubmit={event => {
                void form.handleSubmit(submit)(event)
              }}
            >
              <div className="space-y-2">
                <Label htmlFor="account-reset-email">Email</Label>
                <Input
                  id="account-reset-email"
                  value={preview.email}
                  readOnly
                  disabled
                  autoComplete="username"
                />
              </div>
              <NewPasswordFields
                control={form.control}
                name="password"
                confirmName="confirmPassword"
                disabled={busy}
              />
              <FormField
                control={form.control}
                name="revokeApiAccess"
                render={({ field }) => (
                  <FormItem className="flex flex-row items-start gap-3">
                    <FormControl>
                      <Checkbox
                        checked={field.value}
                        onCheckedChange={checked => {
                          field.onChange(checked === true)
                        }}
                        disabled={busy}
                      />
                    </FormControl>
                    <div className="space-y-1 leading-none">
                      <FormLabel>
                        Also revoke API keys and connected tools
                      </FormLabel>
                      <FormDescription>
                        Deletes your API keys and disconnects MCP and other
                        connected tools. Use it if you think someone else had
                        access.
                      </FormDescription>
                    </div>
                  </FormItem>
                )}
              />
              {rootError && (
                <p
                  role="alert"
                  className="text-destructive text-sm"
                  data-testid="account-reset-error"
                >
                  {rootError}
                </p>
              )}
              <Button type="submit" className="w-full" disabled={busy}>
                {busy ? 'Saving…' : 'Set password and sign in'}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
    </PublicCardPage>
  )
}
