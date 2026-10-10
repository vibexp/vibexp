import { ownerEmailParam } from '@/pages/admin/filters/advancedFilterParams'
import type {
  AdminAuthProvider,
  AdminAuthProviderCreate,
  AdminAuthProviderTestRequest,
  AdminAuthProviderType,
  AdminAuthProviderUpdate,
} from '@/services/authSettingsService'
import { ApiError } from '@/types/errors'

import type { FieldErrors } from '../instanceSettingsForm'

/**
 * Form state, validation and wire mapping for Admin → Settings →
 * Authentication (#1239). A data module rather than helpers in a `.tsx`, like
 * `instanceSettingsForm.ts`. Form keys are the wire field names, so a server
 * field error lands on its input without a mapping table.
 *
 * The client-side rules mirror the server's
 * (`internal/services/instance_auth_settings_validation.go`); the server stays
 * the authority, and its 400 field errors are shown on the same inputs.
 */

/** What to show for a failure: the error's own message, or `fallback`. */
export function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

export const PROVIDER_TYPES: readonly AdminAuthProviderType[] = [
  'google',
  'github',
  'oidc',
]

export const PROVIDER_TYPE_LABELS: Record<AdminAuthProviderType, string> = {
  google: 'Google',
  github: 'GitHub',
  oidc: 'OpenID Connect',
}

/** The provider fields a server validation error can name. */
export const PROVIDER_FORM_FIELDS = [
  'type',
  'slug',
  'display_name',
  'client_id',
  'client_secret',
  'issuer_url',
] as const

export type ProviderField = (typeof PROVIDER_FORM_FIELDS)[number]

export interface ProviderForm {
  type: AdminAuthProviderType
  slug: string
  display_name: string
  client_id: string
  /** Write-only: always starts empty, and blank on an edit keeps the stored one. */
  client_secret: string
  issuer_url: string
  enabled: boolean
}

/** Mirrors the CHECK on `instance_auth_providers.slug`. */
const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{0,62}$/
const DOMAIN_LABEL_CHARS = /^[a-z0-9-]{1,63}$/
const MAX_DOMAIN_LENGTH = 253

/**
 * The types a new provider may still take: Google and GitHub are limited to
 * one provider each, OIDC is not.
 */
export function availableProviderTypes(
  providers: readonly Pick<AdminAuthProvider, 'type'>[]
): AdminAuthProviderType[] {
  return PROVIDER_TYPES.filter(
    type => type === 'oidc' || !providers.some(p => p.type === type)
  )
}

/** A blank form for a new provider of `type`, named after the type. */
export function emptyProviderForm(type: AdminAuthProviderType): ProviderForm {
  return {
    type,
    slug: type === 'oidc' ? '' : type,
    display_name: type === 'oidc' ? '' : PROVIDER_TYPE_LABELS[type],
    client_id: '',
    client_secret: '',
    issuer_url: '',
    enabled: true,
  }
}

export function toProviderForm(provider: AdminAuthProvider): ProviderForm {
  return {
    type: provider.type,
    slug: provider.slug,
    display_name: provider.display_name,
    client_id: provider.client_id,
    client_secret: '',
    issuer_url: provider.issuer_url ?? '',
    enabled: provider.enabled,
  }
}

/**
 * Why `raw` is not an acceptable issuer URL, or null. Absolute, no
 * credentials, query or fragment, and https (http only on localhost).
 */
export function issuerUrlError(raw: string): string | null {
  const value = raw.trim()
  if (value === '') return 'Enter the issuer URL.'
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return 'Enter an absolute URL, such as https://example.okta.com.'
  }
  if (url.host === '') {
    return 'Enter an absolute URL, such as https://example.okta.com.'
  }
  if (
    url.username !== '' ||
    url.password !== '' ||
    value.includes('?') ||
    value.includes('#')
  ) {
    return 'The issuer URL must not carry credentials, a query or a fragment.'
  }
  const loopback =
    url.hostname.toLowerCase() === 'localhost' || url.hostname === '127.0.0.1'
  if (url.protocol === 'https:' || (url.protocol === 'http:' && loopback)) {
    return null
  }
  return 'The issuer URL must use https (http is allowed only for localhost).'
}

/**
 * Whether an edit of `stored` changes its issuer URL — the one edit across
 * which the stored client secret is not kept.
 */
export function issuerChanged(
  form: Pick<ProviderForm, 'type' | 'issuer_url'>,
  stored: Pick<AdminAuthProvider, 'issuer_url'>
): boolean {
  return (
    form.type === 'oidc' && form.issuer_url.trim() !== (stored.issuer_url ?? '')
  )
}

/**
 * Client-side errors of a provider form. `stored` is the provider being
 * edited, or null for a new one. The client secret is required for a new
 * provider and when an edit changes the issuer URL; otherwise blank keeps it.
 */
export function validateProviderForm(
  form: ProviderForm,
  stored: AdminAuthProvider | null
): FieldErrors<ProviderField> {
  const errors: FieldErrors<ProviderField> = {}
  if (stored === null && !SLUG_PATTERN.test(form.slug)) {
    errors.slug =
      'Use 1-63 lower-case letters, digits or hyphens, not starting with a hyphen.'
  }
  if (form.display_name.trim() === '') {
    errors.display_name = 'Enter the name shown on the sign-in button.'
  }
  if (form.client_id.trim() === '') {
    errors.client_id = 'Enter the client ID.'
  }
  if (form.type === 'oidc') {
    const issuer = issuerUrlError(form.issuer_url)
    if (issuer) errors.issuer_url = issuer
  }
  if (form.client_secret === '') {
    if (stored === null) {
      errors.client_secret = 'Enter the client secret.'
    } else if (issuerChanged(form, stored)) {
      errors.client_secret =
        'Enter the client secret again: it is not kept when the issuer URL changes.'
    }
  }
  return errors
}

export function toProviderCreate(
  form: ProviderForm,
  sortOrder: number,
  expectedVersion: number
): AdminAuthProviderCreate {
  return {
    type: form.type,
    slug: form.slug,
    display_name: form.display_name.trim(),
    enabled: form.enabled,
    sort_order: sortOrder,
    client_id: form.client_id.trim(),
    client_secret: form.client_secret,
    ...(form.type === 'oidc' ? { issuer_url: form.issuer_url.trim() } : {}),
    expected_version: expectedVersion,
  }
}

/**
 * The whole-row PUT body for `provider` with `changes` applied. The client
 * secret is sent only when one is given, so an omitted one keeps the stored
 * secret.
 */
export function toProviderUpdate(
  provider: AdminAuthProvider,
  changes: Partial<
    Pick<
      ProviderForm,
      'display_name' | 'client_id' | 'client_secret' | 'issuer_url' | 'enabled'
    > & { sort_order: number }
  >,
  expectedVersion: number,
  confirmLockoutRisk: boolean
): AdminAuthProviderUpdate {
  const issuer = (changes.issuer_url ?? provider.issuer_url ?? '').trim()
  return {
    display_name: (changes.display_name ?? provider.display_name).trim(),
    enabled: changes.enabled ?? provider.enabled,
    sort_order: changes.sort_order ?? provider.sort_order,
    client_id: (changes.client_id ?? provider.client_id).trim(),
    ...(changes.client_secret ? { client_secret: changes.client_secret } : {}),
    ...(provider.type === 'oidc' ? { issuer_url: issuer } : {}),
    expected_version: expectedVersion,
    confirm_lockout_risk: confirmLockoutRisk,
  }
}

/**
 * What the Test button sends: the stored provider's id plus only the fields
 * the form changed, or the full candidate for a provider not saved yet.
 */
export function toProviderTest(
  form: ProviderForm,
  stored: AdminAuthProvider | null
): AdminAuthProviderTestRequest {
  const secret =
    form.client_secret === '' ? {} : { client_secret: form.client_secret }
  if (stored === null) {
    return {
      type: form.type,
      client_id: form.client_id.trim(),
      ...secret,
      ...(form.type === 'oidc' ? { issuer_url: form.issuer_url.trim() } : {}),
    }
  }
  return {
    id: stored.id,
    ...(form.client_id.trim() === stored.client_id
      ? {}
      : { client_id: form.client_id.trim() }),
    ...secret,
    ...(issuerChanged(form, stored)
      ? { issuer_url: form.issuer_url.trim() }
      : {}),
  }
}

/** The `sort_order` that puts a new provider last. */
export function nextSortOrder(
  providers: readonly Pick<AdminAuthProvider, 'sort_order'>[]
): number {
  return providers.reduce((max, p) => Math.max(max, p.sort_order + 1), 0)
}

/** Providers in sign-in page order. */
export function sortProviders<T extends Pick<AdminAuthProvider, 'sort_order'>>(
  providers: readonly T[]
): T[] {
  return [...providers].sort((a, b) => a.sort_order - b.sort_order)
}

/** `providers` with the one at `index` moved one place up (-1) or down (+1). */
export function moveProvider<T>(
  providers: readonly T[],
  index: number,
  direction: -1 | 1
): T[] {
  const target = index + direction
  if (index < 0 || index >= providers.length) return [...providers]
  if (target < 0 || target >= providers.length) return [...providers]
  const next = [...providers]
  const [moved] = next.splice(index, 1)
  next.splice(target, 0, moved)
  return next
}

/** The path every provider's redirect URI ends in. */
const CALLBACK_PATH = '/api/v1/auth/callback'

/**
 * The redirect URI to register with the identity provider. The server derives
 * it from the instance's own URL and returns it on every stored provider; it
 * is the same for all of them. With no provider stored yet there is nothing to
 * read it from, so it is derived from the address the admin is on.
 */
export function redirectUriFor(
  providers: readonly Pick<AdminAuthProvider, 'redirect_uri'>[]
): { value: string; derived: boolean } {
  const known = providers.find(provider => provider.redirect_uri !== '')
  if (known) return { value: known.redirect_uri, derived: false }
  return {
    value: new URL(CALLBACK_PATH, globalThis.location.origin).href,
    derived: true,
  }
}

// --- lockout guard -----------------------------------------------------------

/** The error code of a provider change refused because it could lock sign-in out. */
export const LOCKOUT_RISK_CODE = 'lockout_risk'

/**
 * The machine-readable reason of a 409 `lockout_risk`, or null when `err` is
 * anything else. An unrecognised reason is returned as-is, so a reason added
 * server-side still opens the confirmation (with the generic copy).
 */
export function lockoutRiskReason(err: unknown): string | null {
  if (
    !(err instanceof ApiError) ||
    err.status !== 409 ||
    err.code !== LOCKOUT_RISK_CODE
  ) {
    return null
  }
  const reason = err.metadata?.reason
  return typeof reason === 'string' && reason !== '' ? reason : 'unknown'
}

/** What the lockout confirmation says for `reason`. */
export function lockoutRiskCopy(reason: string): {
  title: string
  description: string
} {
  switch (reason) {
    case 'no_enabled_provider':
      return {
        title: 'This leaves no way to sign in',
        description:
          'After this change no sign-in provider is enabled, so nobody can sign in, including you once your session ends. The instance returns to setup mode, and the only way back in is the setup URL from the server logs or `vibexp admin auth setup rearm`.',
      }
    case 'own_provider':
      return {
        title: 'This is the provider you signed in with',
        description:
          'After this change you cannot sign in through it again once your session ends. Make sure another enabled provider lets you in before you continue.',
      }
    default:
      return {
        title: 'This change could lock you out',
        description:
          'The server refused this change because it could leave you unable to sign in. Continue only if you have another way in.',
      }
  }
}

// --- access allowlist --------------------------------------------------------

export type AllowlistField = 'domains' | 'emails'

export const ALLOWLIST_FORM_FIELDS: readonly AllowlistField[] = [
  'domains',
  'emails',
]

export interface AllowlistForm {
  domains: string[]
  emails: string[]
}

/** Trimmed, lower-cased and de-duplicated, the way the server stores a list. */
export function normalizeAllowlistEntries(
  entries: readonly string[]
): string[] {
  const seen = new Set<string>()
  for (const entry of entries) {
    const value = entry.trim().toLowerCase()
    if (value !== '') seen.add(value)
  }
  return [...seen]
}

/** One DNS label: letters, digits and inner hyphens, 1-63 characters. */
function isDomainLabel(label: string): boolean {
  return (
    DOMAIN_LABEL_CHARS.test(label) &&
    !label.startsWith('-') &&
    !label.endsWith('-')
  )
}

/** Whether `domain` is an email domain: a DNS name of at least two labels. */
export function isAllowlistDomain(domain: string): boolean {
  if (domain.length > MAX_DOMAIN_LENGTH) return false
  const labels = domain.split('.')
  return labels.length >= 2 && labels.every(isDomainLabel)
}

/** Whether `email` is a bare address whose domain is an email domain. */
export function isAllowlistEmail(email: string): boolean {
  if (ownerEmailParam(email) === undefined) return false
  return isAllowlistDomain(email.slice(email.lastIndexOf('@') + 1))
}

/** The first invalid entry of each list, quoted, as the server reports it. */
export function validateAllowlistForm(
  form: AllowlistForm
): FieldErrors<AllowlistField> {
  const errors: FieldErrors<AllowlistField> = {}
  const badDomain = normalizeAllowlistEntries(form.domains).find(
    domain => !isAllowlistDomain(domain)
  )
  if (badDomain !== undefined) {
    errors.domains = `"${badDomain}" is not a domain. Enter it like example.com, without an @.`
  }
  const badEmail = normalizeAllowlistEntries(form.emails).find(
    email => !isAllowlistEmail(email)
  )
  if (badEmail !== undefined) {
    errors.emails = `"${badEmail}" is not a full email address.`
  }
  return errors
}

/** Whether two lists hold the same entries once normalized, in any order. */
export function sameAllowlistEntries(
  a: readonly string[],
  b: readonly string[]
): boolean {
  const left = normalizeAllowlistEntries(a).sort()
  const right = normalizeAllowlistEntries(b).sort()
  return left.length === right.length && left.every((v, i) => v === right[i])
}

/** "N signed-in users will be signed out", with the singular spelled out. */
export function allowlistImpactSentence(count: number): string {
  return count === 1
    ? '1 signed-in user will be signed out'
    : `${String(count)} signed-in users will be signed out`
}
