import { zodResolver } from '@hookform/resolvers/zod'
import { AlertCircle, Loader2, UserPlus } from 'lucide-react'
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
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { NewPasswordFields } from '@/features/password/NewPasswordFields'
import {
  newPasswordFields,
  passwordsMatch,
} from '@/features/password/newPasswordSchema'
import { PublicCardPage } from '@/features/site-info/PublicPageBrand'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'
import { formatDate } from '@/lib/time'
import type { AuthProvider } from '@/services/authService'
import {
  type InvitationPreview,
  publicInvitationService,
} from '@/services/publicInvitationService'
import { ApiError } from '@/types/errors'
import { sessionStore } from '@/utils/storage'

import { ProviderChoices } from '../ProviderChoices'
import { GoToSignIn, InstanceName } from '../PublicCardParts'
import { enabledProviders, messageOf, optionalNameField } from '../publicForms'

const schema = z
  .object({ name: optionalNameField, ...newPasswordFields })
  .superRefine(passwordsMatch)

type RegisterValues = z.infer<typeof schema>

type View =
  | { kind: 'loading' }
  | { kind: 'invalid' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; preview: InvitationPreview; providers: AuthProvider[] }
  /** Signed in, but the invitation is still pending. */
  | { kind: 'waiting'; email: string }

const INVALID_TITLE = 'This invitation link is not valid'
const INVALID_TEXT =
  'It may have expired, been used already, or been withdrawn. Ask the person who invited you to send a new invitation.'

function codeOf(err: unknown): string | null {
  return err instanceof ApiError ? err.code : null
}

function statusOf(err: unknown): number | null {
  return err instanceof ApiError ? err.status : null
}

/** Clears the stash once it is no longer needed: the person is signed in. */
function dropStash(): void {
  sessionStore.remove(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
}

/**
 * The invitation link seen signed out (#1440): what the invitation is, and how
 * to take it up. With an account: sign in, then the existing signed-in accept
 * takes over. Without one: create it with a password (when the instance offers
 * password sign-in) or through a sign-in provider, which comes back here and
 * accepts.
 *
 * `go` performs a full page load (injectable for tests): after the sign-in the
 * app must start again to read the new session.
 */
export function SignedOutInvitation({
  token,
  go = url => {
    globalThis.location.assign(url)
  },
}: Readonly<{ token: string; go?: (url: string) => void }>) {
  const [view, setView] = useState<View>({ kind: 'loading' })

  const load = useCallback(async () => {
    setView({ kind: 'loading' })
    try {
      const [preview, providers] = await Promise.all([
        publicInvitationService.preview(token),
        enabledProviders(),
      ])
      setView({ kind: 'ready', preview, providers })
    } catch (err) {
      // 400: a token that cannot even be a token (bad encoding).
      if (codeOf(err) === 'invitation_invalid' || statusOf(err) === 400) {
        setView({ kind: 'invalid' })
        return
      }
      setView({
        kind: 'error',
        message: messageOf(err, 'The invitation could not be loaded'),
      })
    }
  }, [token])

  useEffect(() => {
    void load()
  }, [load])

  // Where a sign-in round trip (provider or sign-in page) comes back to: the
  // signed-in accept page, through InvitationResumeAfterAuth.
  const stash = () => {
    sessionStore.set(STORAGE_KEYS.PENDING_INVITATION_TOKEN, token)
  }

  if (view.kind === 'loading') {
    return (
      <PublicCardPage>
        <div className="flex flex-col items-center gap-3 text-center">
          <Loader2 className="text-primary size-10 animate-spin" />
          <p className="text-muted-foreground text-sm">Loading invitation…</p>
        </div>
      </PublicCardPage>
    )
  }

  if (view.kind === 'invalid' || view.kind === 'error') {
    return (
      <PublicCardPage>
        <Card className="w-full max-w-md" data-testid="invitation-invalid">
          <CardContent className="flex flex-col gap-4 p-6">
            <h1 className="sr-only">Team invitation</h1>
            <InstanceName />
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertTitle>
                {view.kind === 'invalid'
                  ? INVALID_TITLE
                  : 'The invitation could not be loaded'}
              </AlertTitle>
              <AlertDescription>
                {view.kind === 'invalid' ? INVALID_TEXT : view.message}
              </AlertDescription>
            </Alert>
            {view.kind === 'error' && (
              <Button
                variant="outline"
                onClick={() => {
                  void load()
                }}
              >
                Try again
              </Button>
            )}
            <GoToSignIn />
          </CardContent>
        </Card>
      </PublicCardPage>
    )
  }

  if (view.kind === 'waiting') {
    return (
      <PublicCardPage>
        <Card className="w-full max-w-md" data-testid="invitation-waiting">
          <CardHeader>
            <CardTitle as="h1">Your account is ready</CardTitle>
            <CardDescription>
              You are signed in as {view.email}.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm">
              Joining the team did not finish, so the invitation is still
              waiting for you. Open it to accept.
            </p>
            <Button
              className="w-full"
              onClick={() => {
                go(`/invitations/accept/${encodeURIComponent(token)}`)
              }}
            >
              Open the invitation
            </Button>
          </CardContent>
        </Card>
      </PublicCardPage>
    )
  }

  return (
    <InvitationCard
      preview={view.preview}
      providers={view.providers}
      token={token}
      stash={stash}
      go={go}
      onAccountExists={() => {
        setView({
          ...view,
          preview: {
            ...view.preview,
            account_exists: true,
            password_registration: false,
          },
        })
      }}
      onWaiting={email => {
        setView({ kind: 'waiting', email })
      }}
      onInvalid={() => {
        setView({ kind: 'invalid' })
      }}
    />
  )
}

function InvitationCard({
  preview,
  providers,
  token,
  stash,
  go,
  onAccountExists,
  onWaiting,
  onInvalid,
}: Readonly<{
  preview: InvitationPreview
  providers: AuthProvider[]
  token: string
  stash: () => void
  go: (url: string) => void
  onAccountExists: () => void
  onWaiting: (email: string) => void
  onInvalid: () => void
}>) {
  const { displayName } = useSiteInfo()

  return (
    <PublicCardPage>
      <Card className="w-full max-w-lg" data-testid="invitation-preview">
        <CardHeader className="text-center">
          <div className="bg-primary text-primary-foreground mx-auto mb-2 flex size-12 items-center justify-center rounded-full">
            <UserPlus className="size-6" />
          </div>
          <CardTitle as="h1">Team invitation</CardTitle>
          <CardDescription>
            You&apos;ve been invited to join a team on {displayName}.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <dl className="bg-muted/40 space-y-3 rounded-lg border p-4 text-sm">
            <InfoRow label="Team">
              <span className="font-medium">{preview.team_name}</span>
            </InfoRow>
            {preview.inviter_name && (
              <>
                <Separator />
                <InfoRow label="Invited by">{preview.inviter_name}</InfoRow>
              </>
            )}
            <Separator />
            <InfoRow label="Invited email">{preview.email}</InfoRow>
            <Separator />
            <InfoRow label="Role">
              <span className="capitalize">{preview.role}</span>
            </InfoRow>
            <Separator />
            <InfoRow label="Expires">{formatDate(preview.expires_at)}</InfoRow>
          </dl>

          {preview.account_exists ? (
            <div className="space-y-3" data-testid="invitation-sign-in">
              <p className="text-sm">
                An account for {preview.email} already exists. Sign in to accept
                the invitation.
              </p>
              <Button
                className="w-full"
                onClick={() => {
                  stash()
                  go('/login')
                }}
              >
                Sign in to accept
              </Button>
            </div>
          ) : (
            <CreateAccount
              preview={preview}
              providers={providers}
              token={token}
              stash={stash}
              go={go}
              onAccountExists={onAccountExists}
              onWaiting={onWaiting}
              onInvalid={onInvalid}
            />
          )}
        </CardContent>
      </Card>
    </PublicCardPage>
  )
}

function CreateAccount({
  preview,
  providers,
  token,
  stash,
  go,
  onAccountExists,
  onWaiting,
  onInvalid,
}: Readonly<{
  preview: InvitationPreview
  providers: AuthProvider[]
  token: string
  stash: () => void
  go: (url: string) => void
  onAccountExists: () => void
  onWaiting: (email: string) => void
  onInvalid: () => void
}>) {
  const form = useForm<RegisterValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', password: '', confirmPassword: '' },
  })
  const [busy, setBusy] = useState(false)

  const submit = async (values: RegisterValues) => {
    setBusy(true)
    try {
      const result = await publicInvitationService.register(token, {
        password: values.password,
        ...(values.name ? { name: values.name } : {}),
      })
      // Signed in now: a provider attempt's stash must not send the person
      // back to this invitation (InvitationResumeAfterAuth).
      dropStash()
      if (result.invitation_accepted) {
        // The in-app handshake switches to the team and welcomes the person.
        sessionStore.set(STORAGE_KEYS.INVITATION_JUST_ACCEPTED, {
          team_id: result.team_id,
          team_name: preview.team_name,
        })
        go(`/teams/${result.team_id}`)
        return
      }
      onWaiting(result.user.email)
    } catch (err) {
      setBusy(false)
      const code = codeOf(err)
      if (code === 'account_exists') {
        onAccountExists()
        return
      }
      if (code === 'invitation_invalid') {
        // Withdrawn or used since the preview: the form cannot succeed.
        onInvalid()
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
      form.setError('root', {
        message: messageOf(err, 'Your account could not be created'),
      })
    }
  }

  const rootError = form.formState.errors.root?.message
  const passwordOffered = preview.password_registration

  return (
    <div className="space-y-4" data-testid="invitation-create">
      <p className="text-sm">
        Create your account for <strong>{preview.email}</strong> to join.
      </p>
      {passwordOffered && (
        <Form {...form}>
          <form
            aria-label="Create your account"
            className="space-y-4"
            noValidate
            onSubmit={event => {
              void form.handleSubmit(submit)(event)
            }}
          >
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
                data-testid="invitation-register-error"
              >
                {rootError}
              </p>
            )}
            <Button type="submit" className="w-full" disabled={busy}>
              {busy ? 'Creating your account…' : 'Create account and join'}
            </Button>
          </form>
        </Form>
      )}
      {passwordOffered && providers.length > 0 && (
        <div className="text-muted-foreground flex items-center gap-3 text-xs">
          <Separator className="flex-1" />
          or
          <Separator className="flex-1" />
        </div>
      )}
      <ProviderChoices
        providers={providers}
        beforeSignIn={stash}
        disabled={busy}
      />
      {!passwordOffered && providers.length === 0 && (
        <p className="text-muted-foreground text-sm">
          This instance offers no way to create an account right now. Ask the
          person who invited you.
        </p>
      )}
    </div>
  )
}

function InfoRow({
  label,
  children,
}: Readonly<{ label: string; children: React.ReactNode }>) {
  return (
    <div className="flex flex-col gap-1 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="break-all sm:text-right">{children}</dd>
    </div>
  )
}
