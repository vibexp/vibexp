import type { ReactNode } from 'react'
import type { UseFormReturn } from 'react-hook-form'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { EmailProviderType } from '@/services/emailProviderService'

import {
  type EmailProviderFormValues,
  PROVIDER_TYPES,
  providerTypeMeta,
} from './emailProviderForm'

/**
 * The editable halves of an email-provider form, shared by the team page and
 * the instance admin's Settings → Email page (#1191).
 *
 * Split out of the pages to keep them under the project's `max-lines` /
 * `max-lines-per-function` limits, which are ERRORS here. These are
 * presentational: all state, validation and submission stay on the page.
 *
 * Generic over the form's values so a page may extend the shared fields (the
 * instance form adds two). The cards only ever read and write the shared keys,
 * which every `T` carries, so narrowing the form to them once is sound —
 * react-hook-form's `UseFormReturn` is invariant, which is why it takes a cast.
 */

interface FieldsProps<T extends EmailProviderFormValues> {
  form: UseFormReturn<T>
  busy: boolean
}

const sharedForm = <T extends EmailProviderFormValues>(
  form: UseFormReturn<T>
): UseFormReturn<EmailProviderFormValues> =>
  form as unknown as UseFormReturn<EmailProviderFormValues>

const TEAM_PROVIDER_DESCRIPTION =
  "Choose where this team's mail is sent from. Only the fields for the selected provider are sent."

const TEAM_STORED_CREDENTIAL_HINT =
  'A credential is stored. Leave this blank to keep it; sending a test always requires re-entering it.'

/** Provider type, its per-type non-secret fields, and the one credential. */
export function ProviderCard<T extends EmailProviderFormValues>({
  form: callerForm,
  busy,
  hasCredential,
  description = TEAM_PROVIDER_DESCRIPTION,
  storedCredentialHint = TEAM_STORED_CREDENTIAL_HINT,
  secretHints,
}: Readonly<
  FieldsProps<T> & {
    hasCredential: boolean
    /** Card subtitle; defaults to the team page's copy. */
    description?: ReactNode
    /** Secret-field hint while a credential is stored; defaults to the team page's. */
    storedCredentialHint?: string
    /**
     * Per-type secret-field hints while none is stored, overriding the type's
     * default (the instance says an SMTP credential may be left blank, #1208).
     */
    secretHints?: Partial<Record<EmailProviderType, string>>
  }
>) {
  const form = sharedForm(callerForm)
  const providerType = form.watch('provider_type')
  const meta = providerTypeMeta(providerType)
  const secretHint = secretHints?.[providerType] ?? meta.secretHint

  return (
    <Card>
      <CardHeader>
        <CardTitle>Provider</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <FormField
          control={form.control}
          name="provider_type"
          render={({ field }) => (
            <FormItem>
              {/* Native radios rather than a Radix Select: a Select rendered
                  inside a Dialog crashes jsdom, and the four options each carry
                  a line of explanation that a dropdown could not show. */}
              <fieldset
                className="grid gap-3 sm:grid-cols-2"
                disabled={busy}
                aria-label="Provider type"
              >
                {PROVIDER_TYPES.map(option => (
                  <div
                    key={option.id}
                    className="flex items-start gap-3 rounded-md border p-3"
                  >
                    <input
                      type="radio"
                      id={`provider-${option.id}`}
                      name="provider_type"
                      className="mt-1"
                      value={option.id}
                      checked={field.value === option.id}
                      onChange={() => {
                        field.onChange(option.id)
                      }}
                    />
                    <div className="space-y-1">
                      <Label
                        htmlFor={`provider-${option.id}`}
                        className="block cursor-pointer text-sm font-medium"
                      >
                        {option.label}
                      </Label>
                      <p className="text-muted-foreground text-sm">
                        {option.description}
                      </p>
                    </div>
                  </div>
                ))}
              </fieldset>
              <FormMessage />
            </FormItem>
          )}
        />

        <fieldset className="grid gap-4 sm:grid-cols-2" disabled={busy}>
          {providerType === 'smtp' && (
            <>
              <FormField
                control={form.control}
                name="smtp_host"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Host</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder="smtp.acme.test" />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="smtp_port"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Port</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder="587" />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="smtp_username"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Username</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder="mailer@acme.test" />
                    </FormControl>
                    <FormDescription>
                      Optional — leave blank for a server that does not
                      authenticate.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </>
          )}

          {providerType === 'mailgun' && (
            <>
              <FormField
                control={form.control}
                name="mailgun_domain"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Sending domain</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder="mg.acme.test" />
                    </FormControl>
                    <FormDescription>
                      The bare domain, not a URL.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="mailgun_base_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>API base URL</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder="https://api.eu.mailgun.net/v3"
                      />
                    </FormControl>
                    <FormDescription>
                      Optional — set this for a non-US region.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </>
          )}

          {providerType === 'postmark' && (
            <FormField
              control={form.control}
              name="postmark_message_stream"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Message stream</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder="outbound" />
                  </FormControl>
                  <FormDescription>
                    Optional — defaults to the “outbound” transactional stream.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          <FormField
            control={form.control}
            name="secret"
            render={({ field }) => (
              <FormItem className="sm:col-span-2">
                <FormLabel>{meta.secretLabel}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type="password"
                    autoComplete="new-password"
                    placeholder={
                      hasCredential
                        ? 'Leave blank to keep current key'
                        : secretHint
                    }
                  />
                </FormControl>
                <FormDescription>
                  {hasCredential ? storedCredentialHint : secretHint}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </fieldset>
      </CardContent>
    </Card>
  )
}

/**
 * From address, display name and Reply-To, plus any page-specific fields
 * passed as `children` (rendered in the same grid).
 */
export function SenderIdentityCard<T extends EmailProviderFormValues>({
  form: callerForm,
  busy,
  children,
}: Readonly<FieldsProps<T> & { children?: ReactNode }>) {
  const form = sharedForm(callerForm)
  return (
    <Card>
      <CardHeader>
        <CardTitle>Sender identity</CardTitle>
        <CardDescription>
          The from address must be one your provider is authorized to send for,
          or every message will bounce.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <fieldset className="grid gap-4 sm:grid-cols-2" disabled={busy}>
          <FormField
            control={form.control}
            name="from_address"
            render={({ field }) => (
              <FormItem>
                <FormLabel>From address</FormLabel>
                <FormControl>
                  <Input {...field} placeholder="hello@acme.test" />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="from_name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Display name</FormLabel>
                <FormControl>
                  <Input {...field} placeholder="Acme Team" />
                </FormControl>
                <FormDescription>Optional.</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="reply_to"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Reply-To</FormLabel>
                <FormControl>
                  <Input {...field} placeholder="support@acme.test" />
                </FormControl>
                <FormDescription>Optional.</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          {children}
        </fieldset>
      </CardContent>
    </Card>
  )
}
