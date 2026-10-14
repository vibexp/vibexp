import type { Control, FieldValues, Path } from 'react-hook-form'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { PASSWORD_MAX_LENGTH, PASSWORD_RULE } from '@/lib/passwordPolicy'

/**
 * A new password and its repeat, with the written rule under the first (epic
 * #1426). It only displays: the page validates with `newPasswordSchema` and
 * places the server's field errors with `form.setError(name, …)`. There is no
 * `autoFocus` (jsx-a11y forbids it); a page that wants the field focused calls
 * `form.setFocus(name)`.
 */
export function NewPasswordFields<T extends FieldValues>({
  control,
  name,
  confirmName,
  label = 'Password',
  disabled,
}: Readonly<{
  control: Control<T>
  name: Path<T>
  confirmName: Path<T>
  label?: string
  disabled?: boolean
}>) {
  return (
    <>
      <FormField
        control={control}
        name={name}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{label}</FormLabel>
            <FormControl>
              <Input
                {...field}
                type="password"
                autoComplete="new-password"
                maxLength={PASSWORD_MAX_LENGTH}
                disabled={disabled}
              />
            </FormControl>
            <FormDescription>{PASSWORD_RULE}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name={confirmName}
        render={({ field }) => (
          <FormItem>
            <FormLabel>Repeat the password</FormLabel>
            <FormControl>
              <Input
                {...field}
                type="password"
                autoComplete="new-password"
                maxLength={PASSWORD_MAX_LENGTH}
                disabled={disabled}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}
