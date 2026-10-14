import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
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
import { toast } from '@/lib/toast'
import { type Account, accountService } from '@/services/accountService'
import { ApiError } from '@/types/errors'

import { accountErrorMessage, NAME_MAX_LENGTH } from './accountRules'

const schema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Enter your name.')
    .max(NAME_MAX_LENGTH, `Use at most ${String(NAME_MAX_LENGTH)} characters.`),
})

type ProfileValues = z.infer<typeof schema>

/**
 * The display name, and the email address shown read-only. `onSaved` receives
 * the updated account so the page and the user menu can follow.
 */
export function ProfileCard({
  account,
  onSaved,
}: Readonly<{
  account: Account
  onSaved: (account: Account) => void | Promise<void>
}>) {
  const form = useForm<ProfileValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: account.name },
  })

  // Follow the stored name, e.g. after a save trimmed it.
  useEffect(() => {
    form.reset({ name: account.name })
  }, [account.name, form])

  const submit = async (values: ProfileValues) => {
    try {
      const saved = await accountService.updateName(values.name)
      toast.success('Name saved')
      await onSaved(saved)
    } catch (err) {
      const onName =
        err instanceof ApiError ? err.getFieldErrors('name').at(0) : undefined
      form.setError('name', {
        message:
          onName?.message ??
          accountErrorMessage(err, 'Your name could not be saved'),
      })
    }
  }

  const { isDirty, isSubmitting } = form.formState

  return (
    <Card data-testid="account-profile-card">
      <CardHeader>
        <CardTitle>Profile</CardTitle>
        <CardDescription>
          How you appear to the people you work with.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form
            aria-label="Profile"
            className="max-w-md space-y-4"
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
                  <FormLabel>Name</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      autoComplete="name"
                      disabled={isSubmitting}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="space-y-2">
              <Label htmlFor="account-email">Email</Label>
              <Input
                id="account-email"
                value={account.email}
                readOnly
                disabled
                aria-describedby="account-email-hint"
              />
              <p
                id="account-email-hint"
                className="text-muted-foreground text-sm"
              >
                Your email address cannot be changed.
              </p>
            </div>
            <Button type="submit" disabled={!isDirty || isSubmitting}>
              {isSubmitting ? 'Saving…' : 'Save name'}
            </Button>
          </form>
        </Form>
      </CardContent>
    </Card>
  )
}
