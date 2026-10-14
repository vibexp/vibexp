import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { Link } from 'react-router'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { authService } from '@/services/authService'
import { passwordSignInErrorMessage } from '@/utils/authErrors'

const schema = z.object({
  email: z.string().trim().email('Enter your email address.'),
  // Not trimmed: a password is exactly what was typed.
  password: z.string().min(1, 'Enter your password.'),
})

type SignInValues = z.infer<typeof schema>

/**
 * Email and password sign-in on `/login` (#1439). On success the browser loads
 * `returnTo` afresh, as dev login does, so the app starts with the new session.
 * `go` is injectable for tests.
 */
export function PasswordSignInForm({
  returnTo,
  disabled = false,
  onBusyChange,
  go = url => {
    globalThis.location.assign(url)
  },
}: Readonly<{
  returnTo: string
  disabled?: boolean
  /** Told when a sign-in starts and when it fails, so the page can hold
   *  the other ways in meanwhile. */
  onBusyChange?: (busy: boolean) => void
  go?: (url: string) => void
}>) {
  const form = useForm<SignInValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', password: '' },
  })
  const [busy, setBusy] = useState(false)

  const submit = async (values: SignInValues) => {
    setBusy(true)
    onBusyChange?.(true)
    try {
      await authService.passwordLogin(values.email, values.password)
      go(returnTo)
    } catch (err) {
      setBusy(false)
      onBusyChange?.(false)
      form.setError('root', { message: passwordSignInErrorMessage(err) })
      form.setFocus('password')
    }
  }

  const rootError = form.formState.errors.root?.message
  const off = busy || disabled

  return (
    <Form {...form}>
      <form
        aria-label="Password sign-in"
        className="space-y-3"
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
                  disabled={off}
                />
              </FormControl>
              <FormMessage role="alert" />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="password"
          render={({ field }) => (
            <FormItem>
              <div className="flex items-center justify-between">
                <FormLabel>Password</FormLabel>
                <Link
                  to="/forgot-password"
                  className="text-muted-foreground hover:text-foreground text-xs underline-offset-4 hover:underline"
                >
                  Forgot password?
                </Link>
              </div>
              <FormControl>
                <Input
                  {...field}
                  type="password"
                  autoComplete="current-password"
                  disabled={off}
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
            data-testid="password-sign-in-error"
          >
            {rootError}
          </p>
        )}
        <Button type="submit" className="w-full" disabled={off}>
          {busy ? 'Signing you in…' : 'Sign in'}
        </Button>
      </form>
    </Form>
  )
}
