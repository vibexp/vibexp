import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2, MailCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
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
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { PublicCardPage } from '@/features/site-info/PublicPageBrand'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'
import { authService, type ProvidersResponse } from '@/services/authService'
import { ApiError } from '@/types/errors'

import { GoToSignIn, InstanceName } from './PublicCardParts'

const schema = z.object({
  email: z.string().trim().email('Enter your email address.'),
})

type ForgotValues = z.infer<typeof schema>

type View =
  | { kind: 'loading' }
  /** Password sign-in is off, or the methods could not be read. */
  | { kind: 'unavailable' }
  /** No email provider: an administrator resets passwords. */
  | { kind: 'admin' }
  | { kind: 'form' }
  /** The request was accepted; this never says whether an account exists. */
  | { kind: 'sent'; email: string }

function viewOf(methods: ProvidersResponse): View {
  if (!methods.password.enabled) return { kind: 'unavailable' }
  return methods.password.reset === 'email'
    ? { kind: 'form' }
    : { kind: 'admin' }
}

/**
 * `/forgot-password` (#1439). With an email provider, it asks for an address
 * and always answers with the same neutral confirmation (the server decides in
 * the background whether to send anything, #1433). Without one, it says that
 * an administrator resets passwords on this instance.
 */
export function ForgotPasswordPage() {
  const [view, setView] = useState<View>({ kind: 'loading' })

  useEffect(() => {
    let active = true
    authService
      .getSignInMethods()
      .then(methods => {
        if (active) setView(viewOf(methods))
      })
      .catch(() => {
        if (active) setView({ kind: 'unavailable' })
      })
    return () => {
      active = false
    }
  }, [])

  if (view.kind === 'loading') {
    return (
      <PublicCardPage>
        <div className="flex flex-col items-center gap-3 text-center">
          <Loader2 className="text-primary size-10 animate-spin" />
          <p className="text-muted-foreground text-sm">Loading…</p>
        </div>
      </PublicCardPage>
    )
  }

  return (
    <PublicCardPage>
      <Card
        className="w-full max-w-md"
        data-testid={`forgot-password-${view.kind}`}
      >
        <CardHeader>
          <InstanceName />
          <CardTitle as="h1">Forgot your password?</CardTitle>
          {view.kind === 'form' && (
            <CardDescription>
              Enter the email address of your account and we will email you a
              link to set a new password.
            </CardDescription>
          )}
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {view.kind === 'form' && (
            <ForgotForm
              onSent={email => {
                setView({ kind: 'sent', email })
              }}
              onUnavailable={() => {
                setView({ kind: 'unavailable' })
              }}
            />
          )}
          {view.kind === 'sent' && <Sent email={view.email} />}
          {view.kind === 'admin' && <AskAnAdministrator />}
          {view.kind === 'unavailable' && (
            <Alert>
              <AlertTitle>Password sign-in is not available</AlertTitle>
              <AlertDescription>
                This instance does not offer password sign-in at the moment.
                Sign in with one of the options on the sign-in page.
              </AlertDescription>
            </Alert>
          )}
          <GoToSignIn variant={view.kind === 'form' ? 'outline' : 'default'} />
        </CardContent>
      </Card>
    </PublicCardPage>
  )
}

function ForgotForm({
  onSent,
  onUnavailable,
}: Readonly<{
  onSent: (email: string) => void
  /** Password sign-in was switched off after the page loaded. */
  onUnavailable: () => void
}>) {
  const form = useForm<ForgotValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: '' },
  })
  const [busy, setBusy] = useState(false)

  const submit = async (values: ForgotValues) => {
    setBusy(true)
    try {
      await authService.requestPasswordReset(values.email)
      onSent(values.email)
    } catch (err) {
      setBusy(false)
      if (err instanceof ApiError && err.status === 404) {
        onUnavailable()
        return
      }
      form.setError('root', {
        message:
          err instanceof ApiError && err.status === 429
            ? 'Too many requests from this device. Wait a minute, then try again.'
            : 'The request could not be sent. Try again in a moment.',
      })
    }
  }

  const rootError = form.formState.errors.root?.message

  return (
    <Form {...form}>
      <form
        aria-label="Request a password reset"
        className="space-y-4"
        noValidate
        onSubmit={event => {
          void form.handleSubmit(submit)(event)
        }}
      >
        <FormField
          control={form.control}
          name="email"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Email</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type="email"
                  autoComplete="username"
                  disabled={busy}
                />
              </FormControl>
              <FormMessage role="alert" />
            </FormItem>
          )}
        />
        {rootError && (
          <p
            role="alert"
            className="text-destructive text-sm"
            data-testid="forgot-password-error"
          >
            {rootError}
          </p>
        )}
        <Button type="submit" className="w-full" disabled={busy}>
          {busy ? 'Sending…' : 'Email me a reset link'}
        </Button>
      </form>
    </Form>
  )
}

function Sent({ email }: Readonly<{ email: string }>) {
  return (
    <Alert role="status">
      <MailCheck className="size-4" />
      <AlertTitle>Check your email</AlertTitle>
      <AlertDescription>
        If {email} belongs to an account here, we sent it a link to set a new
        password. The link works for 1 hour. If nothing arrives, check your spam
        folder or ask your administrator.
      </AlertDescription>
    </Alert>
  )
}

function AskAnAdministrator() {
  const { supportHref } = useSiteInfo()
  return (
    <Alert>
      <AlertTitle>Ask your administrator</AlertTitle>
      <AlertDescription>
        <p>
          This instance does not send password reset emails. Your administrator
          can give you a link to set a new password.
        </p>
        {supportHref && (
          <p className="mt-2">
            <a
              href={supportHref}
              className="underline underline-offset-4"
              data-testid="forgot-password-support"
            >
              Contact support
            </a>
          </p>
        )}
      </AlertDescription>
    </Alert>
  )
}
