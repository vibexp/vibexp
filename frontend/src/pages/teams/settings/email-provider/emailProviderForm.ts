import { z } from 'zod'

import type {
  EmailProviderType,
  TeamEmailProviderResponse,
  UpsertTeamEmailProviderRequest,
} from '@/services/emailProviderService'

/**
 * Form schema and wire mapping for the team email-provider page.
 *
 * A sibling data module rather than consts inside `EmailProvider.tsx`: a `.tsx`
 * that exports both a component and a value trips
 * `react-refresh/only-export-components` — which is only a WARNING in this
 * project, so it would ship through green CI (#587). Keeping the rules here
 * also makes them testable without rendering the page, exactly as
 * `searchRankingPresets.ts` does for the search settings page.
 */

/** What each provider type calls its single credential, and how to describe it. */
export interface ProviderTypeMeta {
  id: EmailProviderType
  label: string
  description: string
  /** Label for the one secret field — each provider has exactly one. */
  secretLabel: string
  secretHint: string
}

export const PROVIDER_TYPES: readonly ProviderTypeMeta[] = [
  {
    id: 'smtp',
    label: 'SMTP',
    description: 'Any standard SMTP server, including your own mail host.',
    secretLabel: 'SMTP password',
    secretHint: 'The password for the SMTP username above.',
  },
  {
    id: 'mailgun',
    label: 'Mailgun',
    description: 'Send through a Mailgun sending domain.',
    secretLabel: 'Sending key',
    secretHint: 'The Mailgun sending key for this domain.',
  },
  {
    id: 'postmark',
    label: 'Postmark',
    description: 'Send through a Postmark server and message stream.',
    secretLabel: 'Server token',
    secretHint: "The Postmark server's API token.",
  },
  {
    id: 'sendgrid',
    label: 'SendGrid',
    description: 'Send through SendGrid. The API key is its only setting.',
    secretLabel: 'API key',
    secretHint: 'A SendGrid API key with Mail Send permission.',
  },
]

export function providerTypeMeta(id: EmailProviderType): ProviderTypeMeta {
  return PROVIDER_TYPES.find(p => p.id === id) ?? PROVIDER_TYPES[0]
}

/**
 * The form is FLAT — one field per input — while the wire shape nests the
 * non-secret settings under `settings.<type>`. `toRequest` does the nesting, so
 * switching provider type never has to move values between sub-objects.
 */
export const emailProviderSchema = z
  .object({
    provider_type: z.enum(['smtp', 'mailgun', 'postmark', 'sendgrid']),
    from_address: z.email('Enter a valid email address'),
    from_name: z.string().trim().max(255).optional(),
    reply_to: z.string().trim().optional(),
    // Requiredness depends on the ACTION, not the shape: saving an already
    // configured provider may omit it, testing never may. See `secretError`.
    secret: z.string().optional(),
    smtp_host: z.string().trim().optional(),
    smtp_port: z.string().trim().optional(),
    smtp_username: z.string().trim().optional(),
    mailgun_domain: z.string().trim().optional(),
    mailgun_base_url: z.string().trim().optional(),
    postmark_message_stream: z.string().trim().optional(),
  })
  .superRefine((values, ctx) => {
    if (values.reply_to && !z.email().safeParse(values.reply_to).success) {
      ctx.addIssue({
        code: 'custom',
        path: ['reply_to'],
        message: 'Enter a valid email address',
      })
    }

    if (values.provider_type === 'smtp') {
      if (!values.smtp_host) {
        ctx.addIssue({
          code: 'custom',
          path: ['smtp_host'],
          message: 'Host is required',
        })
      }
      const port = Number(values.smtp_port)
      if (!values.smtp_port) {
        ctx.addIssue({
          code: 'custom',
          path: ['smtp_port'],
          message: 'Port is required',
        })
      } else if (!Number.isInteger(port) || port < 1 || port > 65535) {
        ctx.addIssue({
          code: 'custom',
          path: ['smtp_port'],
          message: 'Port must be a number between 1 and 65535',
        })
      }
    }

    if (values.provider_type === 'mailgun') {
      if (!values.mailgun_domain) {
        ctx.addIssue({
          code: 'custom',
          path: ['mailgun_domain'],
          message: 'Domain is required',
        })
      } else if (values.mailgun_domain.includes('/')) {
        // The backend requires a bare sending domain; a pasted dashboard URL is
        // the likeliest mistake, so name it here rather than round-tripping.
        ctx.addIssue({
          code: 'custom',
          path: ['mailgun_domain'],
          message: 'Use the bare domain, not a URL (for example mg.acme.test)',
        })
      }
    }
  })

export type EmailProviderFormValues = z.infer<typeof emailProviderSchema>

export const EMPTY_FORM: EmailProviderFormValues = {
  provider_type: 'smtp',
  from_address: '',
  from_name: '',
  reply_to: '',
  secret: '',
  smtp_host: '',
  smtp_port: '',
  smtp_username: '',
  mailgun_domain: '',
  mailgun_base_url: '',
  postmark_message_stream: '',
}

/**
 * Seeds the form from a GET response.
 *
 * `secret` is ALWAYS reset to `''` — it is write-only and no response can carry
 * it, so the field starts blank and a blank field means "keep the stored one"
 * (`ModelProviderDialog`'s idiom). An unconfigured team gets the empty form
 * rather than a half-populated one; its `effective_from_address` belongs to the
 * instance, not to this team, so pre-filling it would invite an admin to save
 * an address their own provider is not authorized to send for.
 */
export function toFormValues(
  response: TeamEmailProviderResponse
): EmailProviderFormValues {
  if (!response.configured) return EMPTY_FORM

  const settings = response.settings ?? {}
  return {
    provider_type: response.provider_type ?? 'smtp',
    from_address: response.from_address ?? '',
    from_name: response.from_name ?? '',
    reply_to: response.reply_to ?? '',
    secret: '',
    smtp_host: settings.smtp?.host ?? '',
    smtp_port: settings.smtp?.port ?? '',
    smtp_username: settings.smtp?.username ?? '',
    mailgun_domain: settings.mailgun?.domain ?? '',
    mailgun_base_url: settings.mailgun?.base_url ?? '',
    postmark_message_stream: settings.postmark?.message_stream ?? '',
  }
}

/**
 * Builds the wire request, nesting only the settings block that matches the
 * selected type — the backend REJECTS a block belonging to another type rather
 * than ignoring it, so a stale value left over from switching types must not be
 * sent. SendGrid has no block at all; its only configuration is its secret.
 *
 * A blank `secret` is OMITTED, never sent as `""`: the backend rejects an empty
 * string rather than treating it as "clear", so sending one would turn an
 * untouched field into a 400.
 */
export function toRequest(
  values: EmailProviderFormValues
): UpsertTeamEmailProviderRequest {
  const request: UpsertTeamEmailProviderRequest = {
    provider_type: values.provider_type,
    from_address: values.from_address.trim(),
  }

  const fromName = values.from_name?.trim()
  if (fromName) request.from_name = fromName

  const replyTo = values.reply_to?.trim()
  if (replyTo) request.reply_to = replyTo

  const secret = values.secret?.trim()
  if (secret) request.secret = secret

  switch (values.provider_type) {
    case 'smtp':
      request.settings = {
        smtp: {
          host: values.smtp_host?.trim() ?? '',
          port: values.smtp_port?.trim() ?? '',
          ...(values.smtp_username?.trim()
            ? { username: values.smtp_username.trim() }
            : {}),
        },
      }
      break
    case 'mailgun':
      request.settings = {
        mailgun: {
          domain: values.mailgun_domain?.trim() ?? '',
          ...(values.mailgun_base_url?.trim()
            ? { base_url: values.mailgun_base_url.trim() }
            : {}),
        },
      }
      break
    case 'postmark':
      request.settings = values.postmark_message_stream?.trim()
        ? {
            postmark: { message_stream: values.postmark_message_stream.trim() },
          }
        : {}
      break
    case 'sendgrid':
      // No settings block exists for SendGrid.
      break
  }

  return request
}

/**
 * Whether an action is blocked for want of a credential, and why.
 *
 * The two actions differ, which is the whole reason this is a function:
 *  * SAVE may omit the secret on an already-configured team — the stored one is
 *    kept.
 *  * TEST never may. It sends with the values in the request body rather than
 *    the stored configuration, precisely so credentials can be verified before
 *    they are saved, so there is nothing to fall back on.
 */
export function secretError(
  action: 'save' | 'test',
  configured: boolean,
  secret: string | undefined
): string | null {
  if (secret?.trim()) return null
  if (action === 'test') {
    return 'Enter the credential to send a test — the test sends with the values in this form, not the stored credential.'
  }
  return configured ? null : 'A credential is required'
}
