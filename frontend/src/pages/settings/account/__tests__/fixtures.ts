import type {
  Account,
  AccountDisconnectedIdentity,
  AccountIdentity,
} from '@/services/accountService'
import { ApiError } from '@/types/errors'

export function identity(
  overrides: Partial<AccountIdentity> = {}
): AccountIdentity {
  return {
    id: '11111111-1111-4111-8111-111111111111',
    provider: 'google',
    display_name: 'Google',
    available: true,
    connected_at: '2026-09-01T10:00:00Z',
    last_used_at: '2026-10-10T10:00:00Z',
    ...overrides,
  }
}

export function disconnectedIdentity(
  overrides: Partial<AccountDisconnectedIdentity> = {}
): AccountDisconnectedIdentity {
  return {
    id: '33333333-3333-4333-8333-333333333333',
    provider: 'google',
    display_name: 'Google',
    disconnected_at: '2026-10-14T09:00:00Z',
    ...overrides,
  }
}

export function account(overrides: Partial<Account> = {}): Account {
  return {
    id: '22222222-2222-4222-8222-222222222222',
    email: 'ada@example.com',
    name: 'Ada Lovelace',
    password: {
      set: false,
      changed_at: null,
      login_enabled: true,
      disabled: false,
    },
    identities: [identity()],
    disconnected_identities: [],
    recent_sign_in: true,
    ...overrides,
  }
}

export function withPassword(overrides: Partial<Account> = {}): Account {
  return account({
    password: {
      set: true,
      changed_at: '2026-10-01T10:00:00Z',
      login_enabled: true,
      disabled: false,
    },
    ...overrides,
  })
}

export function apiError(
  status: number,
  code: string,
  detail: string,
  validation: { field: string; message: string; code: string }[] = []
): ApiError {
  return new ApiError({
    type: 'about:blank',
    title: 'Refused',
    status,
    detail,
    code,
    request_id: 'req-1',
    timestamp: '2026-10-14T00:00:00Z',
    ...(validation.length > 0 ? { validation_errors: validation } : {}),
  })
}
