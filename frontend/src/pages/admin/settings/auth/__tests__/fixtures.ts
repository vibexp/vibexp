import type {
  AdminAuthAllowlist,
  AdminAuthProvider,
} from '@/services/authSettingsService'
import { ApiError } from '@/types/errors'

export const REDIRECT_URI = 'https://vibexp.example.com/api/v1/auth/callback'

export function provider(
  overrides: Partial<AdminAuthProvider> = {}
): AdminAuthProvider {
  return {
    id: '11111111-1111-4111-8111-111111111111',
    type: 'oidc',
    slug: 'okta',
    display_name: 'Okta',
    enabled: true,
    sort_order: 0,
    client_id: 'client-okta',
    has_client_secret: true,
    issuer_url: 'https://example.okta.com',
    redirect_uri: REDIRECT_URI,
    health: { status: 'healthy', last_error: null, checked_at: null },
    created_at: '2026-10-01T10:00:00Z',
    updated_at: '2026-10-01T10:00:00Z',
    updated_by_user_id: null,
    ...overrides,
  }
}

export const google = provider({
  id: '22222222-2222-4222-8222-222222222222',
  type: 'google',
  slug: 'google',
  display_name: 'Google',
  sort_order: 1,
  client_id: 'client-google',
  issuer_url: null,
})

export const github = provider({
  id: '33333333-3333-4333-8333-333333333333',
  type: 'github',
  slug: 'github',
  display_name: 'GitHub',
  sort_order: 2,
  client_id: 'client-github',
  issuer_url: null,
})

export function allowlist(
  overrides: Partial<AdminAuthAllowlist> = {}
): AdminAuthAllowlist {
  return {
    domains: [],
    emails: [],
    active: false,
    version: null,
    updated_at: null,
    updated_by_user_id: null,
    ...overrides,
  }
}

export function apiError(
  status: number,
  code: string,
  extra: {
    detail?: string
    metadata?: Record<string, unknown>
    validation_errors?: { field: string; message: string }[]
  } = {}
): ApiError {
  return new ApiError({
    type: 'about:blank',
    title: 'Error',
    status,
    detail: extra.detail ?? `${code} detail`,
    code,
    request_id: 'req-1',
    timestamp: '2026-10-10T00:00:00Z',
    ...(extra.metadata ? { metadata: extra.metadata } : {}),
    ...(extra.validation_errors
      ? {
          validation_errors: extra.validation_errors.map(error => ({
            ...error,
            code: 'INVALID_VALUE',
          })),
        }
      : {}),
  })
}

export const versionConflict = () =>
  apiError(409, 'INSTANCE_SETTINGS_VERSION_CONFLICT')

export const lockoutRisk = (reason: string) =>
  apiError(409, 'lockout_risk', { metadata: { reason } })
