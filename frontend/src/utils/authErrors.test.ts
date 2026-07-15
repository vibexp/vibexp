import type { APIErrorResponse } from '@/types/errors'
import { ApiError } from '@/types/errors'

import {
  ACCESS_RESTRICTED_CODE,
  ACCESS_RESTRICTED_ERROR,
  ACCESS_RESTRICTED_MESSAGE,
  GENERIC_AUTH_ERROR,
  mapAuthCallbackError,
  mapSignInError,
} from './authErrors'

const buildApiError = (
  status: number,
  overrides: Partial<APIErrorResponse> = {}
): ApiError =>
  new ApiError({
    type: 'https://api.vibexp.io/errors/test',
    title: 'Test Error',
    status,
    detail: 'detail',
    code: 'TEST',
    request_id: 'req-1',
    timestamp: '2024-01-01T00:00:00Z',
    ...overrides,
  })

describe('mapAuthCallbackError', () => {
  it('maps access_restricted to the restriction view', () => {
    expect(mapAuthCallbackError(ACCESS_RESTRICTED_CODE)).toEqual(
      ACCESS_RESTRICTED_ERROR
    )
  })

  it('falls back to the generic view for unknown, empty and absent codes', () => {
    expect(mapAuthCallbackError('access_denied')).toEqual(GENERIC_AUTH_ERROR)
    expect(mapAuthCallbackError('')).toEqual(GENERIC_AUTH_ERROR)
    expect(mapAuthCallbackError(null)).toEqual(GENERIC_AUTH_ERROR)
  })

  it('does not match the code case-insensitively (it is lowercase by contract)', () => {
    expect(mapAuthCallbackError('ACCESS_RESTRICTED')).toEqual(
      GENERIC_AUTH_ERROR
    )
  })
})

describe('mapSignInError', () => {
  it('maps an access_restricted ApiError to the shared restriction wording', () => {
    const err = buildApiError(403, {
      code: ACCESS_RESTRICTED_CODE,
      detail: 'Your account is not permitted to sign in',
    })

    expect(mapSignInError(err, 'Dev login failed')).toBe(
      ACCESS_RESTRICTED_MESSAGE
    )
  })

  it('keeps the backend detail for any other ApiError', () => {
    const err = buildApiError(500, { detail: 'Something exploded' })

    expect(mapSignInError(err, 'Dev login failed')).toBe('Something exploded')
  })

  it('keeps the message of a plain Error', () => {
    expect(mapSignInError(new Error('network down'), 'Dev login failed')).toBe(
      'network down'
    )
  })

  it('falls back for non-Error values', () => {
    expect(mapSignInError('a string', 'Dev login failed')).toBe(
      'Dev login failed'
    )
    expect(mapSignInError(undefined, 'Dev login failed')).toBe(
      'Dev login failed'
    )
  })
})
