import { z } from 'zod'

import type {
  AdminSiteInfo,
  AdminSiteInfoUpdate,
} from '@/services/adminSettingsService'

import type { AuditField } from '../instanceSettingsAudit'
import type { FieldErrors } from '../instanceSettingsForm'
import type { InstanceSettingsEnvelope } from '../useInstanceSettingsEditor'

/**
 * Form state, validation and wire mapping for Admin → Settings → Site info
 * (#1358). The rules mirror the server's (`ValidateInstanceSiteInfo`) for
 * immediate feedback; the server stays authoritative.
 *
 * Every value is a string and every value is optional: an empty string is
 * the API's "not configured", so a blank field is saved as `""`, never null.
 */

export type SiteInfoValues = AdminSiteInfo['values']
export type SiteInfoLimits = AdminSiteInfo['limits']

/** The form is the wire shape: eight strings under their wire names. */
export type SiteInfoForm = SiteInfoValues
export type SiteInfoFormField = keyof SiteInfoForm

export const URL_FIELDS = [
  'website',
  'support_url',
  'terms_url',
  'privacy_url',
  'logo_url',
  'logo_dark_url',
] as const satisfies readonly SiteInfoFormField[]

export const SITE_INFO_FORM_FIELDS: readonly SiteInfoFormField[] = [
  'company_name',
  'company_email',
  ...URL_FIELDS,
]

export const SITE_INFO_LABELS: Record<SiteInfoFormField, string> = {
  company_name: 'Company name',
  company_email: 'Company email',
  website: 'Website',
  support_url: 'Support URL',
  terms_url: 'Terms of service URL',
  privacy_url: 'Privacy policy URL',
  logo_url: 'Logo URL',
  logo_dark_url: 'Dark-mode logo URL',
}

/** The history's allowlist, in form order. */
export const SITE_INFO_AUDIT_FIELDS: readonly AuditField[] =
  SITE_INFO_FORM_FIELDS.map(field => [field, SITE_INFO_LABELS[field]] as const)

export const EMPTY_SITE_INFO_VALUES: SiteInfoValues = {
  company_name: '',
  company_email: '',
  website: '',
  support_url: '',
  terms_url: '',
  privacy_url: '',
  logo_url: '',
  logo_dark_url: '',
}

export function toSiteInfoForm(values: SiteInfoValues): SiteInfoForm {
  return { ...values }
}

/**
 * The PUT body without `expected_version`. Values are trimmed, as the server
 * trims them, so what is sent is what is stored.
 */
export function toSiteInfoUpdate(
  form: SiteInfoForm
): Omit<AdminSiteInfoUpdate, 'expected_version'> {
  return {
    company_name: form.company_name.trim(),
    company_email: form.company_email.trim(),
    website: form.website.trim(),
    support_url: form.support_url.trim(),
    terms_url: form.terms_url.trim(),
    privacy_url: form.privacy_url.trim(),
    logo_url: form.logo_url.trim(),
    logo_dark_url: form.logo_dark_url.trim(),
  }
}

/** Whether `raw` is an absolute `http`/`https` URL with a host. */
export function isHttpUrl(raw: string): boolean {
  let parsed: URL
  try {
    parsed = new URL(raw)
  } catch {
    return false
  }
  return (
    (parsed.protocol === 'http:' || parsed.protocol === 'https:') &&
    parsed.hostname !== ''
  )
}

/** Length in characters (code points), as the server counts it. */
const runeCount = (value: string): number => Array.from(value).length

const tooLong = (limit: number) => `Enter at most ${String(limit)} characters.`

export function validateSiteInfoForm(
  form: SiteInfoForm,
  limits: SiteInfoLimits
): FieldErrors<SiteInfoFormField> {
  const errors: FieldErrors<SiteInfoFormField> = {}

  const name = form.company_name.trim()
  if (runeCount(name) > limits.company_name_max_length) {
    errors.company_name = tooLong(limits.company_name_max_length)
  }

  const email = form.company_email.trim()
  if (email !== '') {
    if (runeCount(email) > limits.company_email_max_length) {
      errors.company_email = tooLong(limits.company_email_max_length)
    } else if (!z.email().safeParse(email).success) {
      errors.company_email =
        'Enter one email address, such as hello@example.com.'
    }
  }

  for (const field of URL_FIELDS) {
    const value = form[field].trim()
    if (value === '') continue
    if (runeCount(value) > limits.url_max_length) {
      errors[field] = tooLong(limits.url_max_length)
    } else if (!isHttpUrl(value)) {
      errors[field] = 'Enter a full URL starting with http:// or https://.'
    }
  }

  return errors
}

/** What the shared instance-settings editor needs around the values. */
export type SiteInfoEnvelope = InstanceSettingsEnvelope<
  SiteInfoValues,
  SiteInfoLimits
>

/**
 * Site info has no built-in defaults and no team overrides, so its response
 * carries neither. The shared editor expects both; they are filled in here
 * rather than added to the API: "nothing stored" is the default state, and
 * the values it falls back to are all empty.
 */
export function toSiteInfoEnvelope(response: AdminSiteInfo): SiteInfoEnvelope {
  return {
    values: response.values,
    limits: response.limits,
    built_in_defaults: EMPTY_SITE_INFO_VALUES,
    source: response.version === null ? 'default' : 'instance',
    teams_with_override: 0,
    updated_at: response.updated_at,
    updated_by_name: response.updated_by_name,
    version: response.version,
  }
}
