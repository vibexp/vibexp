import { z } from 'zod'

import {
  emailProviderShape,
  refineEmailProvider,
  toFormValues,
  toRequest,
} from '@/features/email-provider/emailProviderForm'
import type {
  AdminInstanceEmailSettings,
  AdminInstanceEmailSettingsRequest,
} from '@/services/adminService'

/**
 * Form schema, wire mapping and secret rules for Admin → Settings → Email
 * (#1191).
 *
 * The provider/sender part is the shared email-provider form
 * (`features/email-provider`), so the four-way provider switch lives in one
 * place. The instance adds two fields the team form does not have, and its
 * test send follows a different secret rule (`instanceSecretError`).
 *
 * A data module rather than consts in a `.tsx` for the same reason as
 * `emailProviderForm.ts` (#587).
 */

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      url.hostname !== ''
    )
  } catch {
    return false
  }
}

export const instanceEmailSchema = z
  .object({
    ...emailProviderShape,
    contact_recipient_address: z.string().trim().optional(),
    privacy_policy_url: z.string().trim().optional(),
  })
  .superRefine((values, ctx) => {
    refineEmailProvider(values, ctx)

    const contact = values.contact_recipient_address
    if (contact && !z.email().safeParse(contact).success) {
      ctx.addIssue({
        code: 'custom',
        path: ['contact_recipient_address'],
        message: 'Enter a valid email address',
      })
    }

    const privacy = values.privacy_policy_url
    if (privacy && !isHttpUrl(privacy)) {
      ctx.addIssue({
        code: 'custom',
        path: ['privacy_policy_url'],
        message: 'Enter an absolute http(s) URL',
      })
    }
  })

export type InstanceEmailFormValues = z.infer<typeof instanceEmailSchema>

/**
 * Seeds the form from the GET response. The secret always starts blank (it is
 * write-only), and an unconfigured instance gets the empty form.
 */
export function toInstanceFormValues(
  settings: AdminInstanceEmailSettings
): InstanceEmailFormValues {
  return {
    ...toFormValues(settings),
    contact_recipient_address: settings.configured
      ? (settings.contact_recipient_address ?? '')
      : '',
    privacy_policy_url: settings.configured
      ? (settings.privacy_policy_url ?? '')
      : '',
  }
}

const blankToNull = (value: string | undefined): string | null =>
  value?.trim() ? value.trim() : null

/**
 * Builds the PUT body (also the candidate body of a test send).
 *
 * The PUT replaces the whole row, so the optional fields are sent as an
 * explicit `null` when blank — clearing a previously stored value is then
 * visible in the request rather than implied by an absent key. The secret is
 * still OMITTED when blank (an empty string is rejected server-side, and
 * omitting it is what keeps the stored credential).
 */
export function toInstanceRequest(
  values: InstanceEmailFormValues
): AdminInstanceEmailSettingsRequest {
  return {
    ...toRequest(values),
    from_name: blankToNull(values.from_name),
    reply_to: blankToNull(values.reply_to),
    contact_recipient_address: blankToNull(values.contact_recipient_address),
    privacy_policy_url: blankToNull(values.privacy_policy_url),
  }
}

/**
 * Whether the form still shows exactly the stored configuration (blank secret
 * included). A test send of an untouched form tests the STORED configuration
 * rather than re-submitting it as a candidate. Compared field by field rather
 * than through react-hook-form's `isDirty`, which is only computed when read
 * during render.
 */
export function formMatchesStored(
  stored: AdminInstanceEmailSettings,
  values: InstanceEmailFormValues
): boolean {
  const seeded = toInstanceFormValues(stored)
  return (Object.keys(seeded) as (keyof InstanceEmailFormValues)[]).every(
    key => (values[key] ?? '') === (seeded[key] ?? '')
  )
}

/**
 * Whether the form targets the destination the stored credential was issued
 * for — the server's `sameStoredDestination`: the same provider type and, where
 * the admin chooses the endpoint, the same SMTP host/port/username or Mailgun
 * domain/base URL. Postmark and SendGrid have fixed vendor endpoints, so the
 * type alone identifies them.
 */
export function sameStoredDestination(
  stored: AdminInstanceEmailSettings,
  values: InstanceEmailFormValues
): boolean {
  if (!stored.configured || stored.provider_type !== values.provider_type) {
    return false
  }
  const same = (a: string | undefined, b: string | undefined) =>
    (a ?? '').trim() === (b ?? '').trim()

  switch (values.provider_type) {
    case 'smtp': {
      const smtp = stored.settings?.smtp
      return (
        same(smtp?.host, values.smtp_host) &&
        same(smtp?.port, values.smtp_port) &&
        same(smtp?.username, values.smtp_username)
      )
    }
    case 'mailgun': {
      const mailgun = stored.settings?.mailgun
      return (
        same(mailgun?.domain, values.mailgun_domain) &&
        same(mailgun?.base_url, values.mailgun_base_url)
      )
    }
    default:
      return true
  }
}

/**
 * Whether the stored credential applies to the provider type selected in the
 * form, i.e. whether a blank credential field keeps it. After a switch to
 * another type it does not: the server never carries a credential across types,
 * so the form must not say "leave blank to keep it" — on a switch to SMTP a blank
 * field configures an unauthenticated relay instead (#1208).
 */
export function storedCredentialApplies(
  stored: AdminInstanceEmailSettings,
  selectedType: InstanceEmailFormValues['provider_type']
): boolean {
  return (
    stored.configured &&
    stored.has_credential &&
    stored.provider_type === selectedType
  )
}

/**
 * Whether a test send of this form runs WITHOUT a credential although saving
 * the same form keeps the stored one (#1222): SMTP, a blank credential field, a
 * stored SMTP credential, and a host, port or username that differs from the
 * stored one. The server does not borrow the stored secret for a changed
 * destination (a test is unaudited), while a save keeps it for the same
 * provider type whatever the host — so a relay without AUTH can test "sent" and
 * then fail real sends, and a host that needs AUTH can fail the test but work
 * once saved. The page says so instead of letting the test mislead.
 */
export function testSkipsStoredCredential(
  stored: AdminInstanceEmailSettings,
  values: InstanceEmailFormValues
): boolean {
  return (
    values.provider_type === 'smtp' &&
    !values.secret?.trim() &&
    storedCredentialApplies(stored, 'smtp') &&
    !sameStoredDestination(stored, values)
  )
}

/**
 * Whether an action is blocked for want of a credential, and why. Mirrors the
 * server's rules so the admin gets an inline error instead of a 400:
 *
 *  * SAVE may omit the secret when a provider of the SAME TYPE is stored — the
 *    stored credential is kept. A new configuration or a change of type needs
 *    its own: the instance stores one secret, and reusing, say, a Mailgun key
 *    as an SMTP password would stop instance mail.
 *  * TEST (of a candidate configuration) may omit it only when the candidate
 *    targets the SAME DESTINATION as the stored one. Unlike the team test, the
 *    stored credential is borrowed in that case — but never for a changed
 *    host/domain, since a test send is unaudited and would otherwise hand the
 *    write-only secret to whatever listener the form points at.
 *  * SMTP never needs one, for either action: a blank credential is an
 *    unauthenticated relay such as Mailpit (#1208). The server stores none and
 *    sends without AUTH (or, same type or destination, keeps/borrows the stored
 *    one as above). Team SMTP still requires a credential.
 */
export function instanceSecretError(
  action: 'save' | 'test',
  stored: AdminInstanceEmailSettings,
  values: InstanceEmailFormValues
): string | null {
  if (values.secret?.trim()) return null
  if (values.provider_type === 'smtp') return null

  if (action === 'test') {
    if (sameStoredDestination(stored, values)) return null
    return stored.configured
      ? 'Enter the credential to test a different destination — the stored one is only used for the destination it was saved for.'
      : 'Enter the credential to send a test.'
  }

  if (!stored.configured) return 'A credential is required'
  if (stored.provider_type !== values.provider_type) {
    return 'Enter the new provider’s credential — the stored one belongs to the provider you are switching away from.'
  }
  return null
}

/**
 * Where a server field error belongs in this form. The API reports nested
 * paths (`settings.smtp.host`) while the form is flat (`smtp_host`); a field
 * the form has no input for (e.g. `settings.smtp`, a whole block) maps to
 * nothing and is reported through the toast instead.
 */
const SERVER_FIELD_TO_FORM: Record<string, keyof InstanceEmailFormValues> = {
  provider_type: 'provider_type',
  secret: 'secret',
  from_address: 'from_address',
  from_name: 'from_name',
  reply_to: 'reply_to',
  contact_recipient_address: 'contact_recipient_address',
  privacy_policy_url: 'privacy_policy_url',
  'settings.smtp.host': 'smtp_host',
  'settings.smtp.port': 'smtp_port',
  'settings.smtp.username': 'smtp_username',
  'settings.mailgun.domain': 'mailgun_domain',
  'settings.mailgun.base_url': 'mailgun_base_url',
  'settings.postmark.message_stream': 'postmark_message_stream',
}

export function formFieldForServerField(
  field: string
): keyof InstanceEmailFormValues | undefined {
  return SERVER_FIELD_TO_FORM[field]
}
