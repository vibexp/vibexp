import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { useForm, type UseFormSetError } from 'react-hook-form'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  PASSWORD_MAX_LENGTH,
  PASSWORD_MIN_LENGTH,
  PASSWORD_RULE,
} from '@/lib/passwordPolicy'
import { toast } from '@/lib/toast'
import { accountService } from '@/services/accountService'
import { ApiError } from '@/types/errors'

import { accountErrorMessage } from './accountRules'
import type { AccountState } from './useAccount'

interface PasswordValues {
  current_password: string
  new_password: string
  repeat: string
  revoke_api_access: boolean
}

const EMPTY: PasswordValues = {
  current_password: '',
  new_password: '',
  repeat: '',
  revoke_api_access: false,
}

/** The fields a server validation error can name. */
const SERVER_FIELDS = ['current_password', 'new_password'] as const

function buildSchema(hasPassword: boolean) {
  return z
    .object({
      current_password: hasPassword
        ? z.string().min(1, 'Enter your current password.')
        : z.string(),
      new_password: z
        .string()
        .min(1, 'Enter a new password.')
        .min(
          PASSWORD_MIN_LENGTH,
          `Use at least ${String(PASSWORD_MIN_LENGTH)} characters.`
        )
        .max(
          PASSWORD_MAX_LENGTH,
          `Use at most ${String(PASSWORD_MAX_LENGTH)} characters.`
        ),
      repeat: z.string(),
      revoke_api_access: z.boolean(),
    })
    .refine(values => values.repeat === values.new_password, {
      path: ['repeat'],
      message: 'The two passwords do not match.',
    })
}

const STALE_PASSWORD_MESSAGE =
  'Your account already has a password. Enter it as the current password to change it.'

/** What the success toast says: what changed and what else it ended. */
function successMessage(hadPassword: boolean, revoked: boolean): string {
  const what = hadPassword ? 'Password changed.' : 'Password set.'
  const ended = revoked
    ? 'Your other devices were signed out, and your API keys and connected tools were revoked.'
    : 'Your other devices were signed out.'
  return `${what} ${ended}`
}

/** The request body: the current password goes along only when there is one. */
function passwordRequest(values: PasswordValues, hasPassword: boolean) {
  return {
    ...(hasPassword ? { current_password: values.current_password } : {}),
    new_password: values.new_password,
    revoke_api_access: values.revoke_api_access,
  }
}

/**
 * Put the server's field errors on their inputs, and say whether any landed.
 * `onStale` runs when the server asks for a current password the page did not
 * know about.
 */
function placeFieldErrors(
  err: unknown,
  hasPassword: boolean,
  setError: UseFormSetError<PasswordValues>,
  onStale: () => void
): boolean {
  if (!(err instanceof ApiError)) return false
  let placed = false
  for (const field of SERVER_FIELDS) {
    const first = err.getFieldErrors(field).at(0)
    if (!first) continue
    placed = true
    if (field === 'current_password' && !hasPassword) {
      // The page thought there was no password and the server has one
      // (set in another tab, or by a change that was half applied).
      // That field is not on screen: say it on the form and read the
      // account again, which brings the right form.
      setError('root', { message: STALE_PASSWORD_MESSAGE })
      onStale()
      continue
    }
    setError(field, { message: first.message })
  }
  return placed
}

/**
 * Set a first password, or change the existing one (#1441). The server's
 * field errors land on their inputs; anything else lands on the form.
 */
export function PasswordForm({
  state,
  hasPassword,
  blocked,
}: Readonly<{
  state: AccountState
  hasPassword: boolean
  /** A first password needs a recent sign-in; the caller explains it. */
  blocked: boolean
}>) {
  const schema = useMemo(() => buildSchema(hasPassword), [hasPassword])
  const form = useForm<PasswordValues>({
    resolver: zodResolver(schema),
    defaultValues: EMPTY,
  })

  const submit = async (values: PasswordValues) => {
    try {
      const sent = await state.change(() =>
        accountService.setPassword(passwordRequest(values, hasPassword))
      )
      if (!sent) return
      form.reset(EMPTY)
      toast.success(successMessage(hasPassword, values.revoke_api_access))
    } catch (err) {
      const placed = placeFieldErrors(err, hasPassword, form.setError, () => {
        void state.reload()
      })
      if (!placed) {
        form.setError('root', {
          message: accountErrorMessage(err, 'The password could not be saved'),
        })
      }
    }
  }

  const disabled = state.busy || blocked
  const rootError = form.formState.errors.root?.message

  return (
    <Form {...form}>
      <form
        aria-label={hasPassword ? 'Change password' : 'Set a password'}
        className="max-w-md space-y-4"
        noValidate
        onSubmit={event => {
          void form.handleSubmit(submit)(event)
        }}
      >
        {hasPassword && (
          <FormField
            control={form.control}
            name="current_password"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Current password</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type="password"
                    autoComplete="current-password"
                    disabled={disabled}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
        <FormField
          control={form.control}
          name="new_password"
          render={({ field }) => (
            <FormItem>
              <FormLabel>New password</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type="password"
                  autoComplete="new-password"
                  disabled={disabled}
                />
              </FormControl>
              <FormDescription>{PASSWORD_RULE}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="repeat"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Repeat the new password</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type="password"
                  autoComplete="new-password"
                  disabled={disabled}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="revoke_api_access"
          render={({ field }) => (
            <FormItem className="flex flex-row items-start gap-2 space-y-0">
              <FormControl>
                <Checkbox
                  checked={field.value}
                  disabled={disabled}
                  onCheckedChange={value => {
                    field.onChange(value === true)
                  }}
                  className="mt-0.5"
                />
              </FormControl>
              <div className="space-y-1 leading-none">
                <FormLabel>Also revoke API keys and connected tools</FormLabel>
                <FormDescription>
                  Deletes your API keys and disconnects your MCP and CLI tools.
                  Use it if someone else may have had access to your account.
                  Your other browser sessions are signed out either way.
                </FormDescription>
              </div>
            </FormItem>
          )}
        />
        {rootError && (
          <p
            role="alert"
            className="text-destructive text-sm"
            data-testid="account-password-error"
          >
            {rootError}
          </p>
        )}
        <Button type="submit" disabled={disabled}>
          {hasPassword ? 'Change password' : 'Set password'}
        </Button>
      </form>
    </Form>
  )
}
