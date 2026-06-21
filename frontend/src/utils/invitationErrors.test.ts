import type { APIErrorResponse } from '@/types/errors'
import { ApiError } from '@/types/errors'

import {
  GENERIC_ACCEPT_ERROR,
  GENERIC_LOAD_ERROR,
  INVALID_LINK_ERROR,
  mapInvitationError,
  SESSION_EXPIRED_ERROR,
} from './invitationErrors'

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

describe('mapInvitationError', () => {
  it('returns generic load error for non-ApiError values', () => {
    expect(mapInvitationError(new Error('boom'))).toEqual(GENERIC_LOAD_ERROR)
    expect(mapInvitationError(undefined)).toEqual(GENERIC_LOAD_ERROR)
    expect(mapInvitationError('a string')).toEqual(GENERIC_LOAD_ERROR)
  })

  it('returns session-expired for 401', () => {
    expect(mapInvitationError(buildApiError(401))).toEqual(
      SESSION_EXPIRED_ERROR
    )
  })

  it('returns not-found for 404', () => {
    const view = mapInvitationError(buildApiError(404))
    expect(view.title).toBe('Invitation not found')
    expect(view.description).toBe('Invitation not found.')
  })

  it('returns expired for 410', () => {
    const view = mapInvitationError(buildApiError(410))
    expect(view.title).toBe('Invitation expired')
    expect(view.description).toBe('This invitation has expired.')
  })

  it('returns revoked for 409 + metadata.status=revoked', () => {
    const view = mapInvitationError(
      buildApiError(409, { metadata: { status: 'revoked' } })
    )
    expect(view.title).toBe('Invitation revoked')
    expect(view.description).toBe('This invitation has been revoked.')
  })

  it('returns already-accepted for 409 + metadata.status=accepted', () => {
    const view = mapInvitationError(
      buildApiError(409, { metadata: { status: 'accepted' } })
    )
    expect(view.title).toBe('Invitation no longer available')
    expect(view.description).toBe('This invitation has already been accepted.')
  })

  it('returns already-rejected for 409 + metadata.status=rejected', () => {
    const view = mapInvitationError(
      buildApiError(409, { metadata: { status: 'rejected' } })
    )
    expect(view.description).toBe('This invitation has already been rejected.')
  })

  it('returns generic "no longer valid" for 409 with no recognized metadata', () => {
    const view = mapInvitationError(buildApiError(409))
    expect(view.description).toBe('This invitation is no longer valid.')
  })

  it('returns generic load error for 5xx and unknown status', () => {
    expect(mapInvitationError(buildApiError(500))).toEqual(GENERIC_LOAD_ERROR)
    expect(mapInvitationError(buildApiError(502))).toEqual(GENERIC_LOAD_ERROR)
    expect(mapInvitationError(buildApiError(418))).toEqual(GENERIC_LOAD_ERROR)
  })

  it('detects email mismatch on 403 by code', () => {
    const view = mapInvitationError(
      buildApiError(403, {
        code: 'INVITATION_EMAIL_MISMATCH',
        metadata: {
          invitee_email: 'invitee@example.com',
          actor_email: 'actor@example.com',
        },
      })
    )
    expect(view.title).toBe('Wrong account')
    expect(view.description).toContain('invitee@example.com')
    expect(view.description).toContain('actor@example.com')
  })

  it('detects email mismatch on 409 by detail heuristic', () => {
    const view = mapInvitationError(
      buildApiError(409, {
        detail: 'Invitee email does not match authenticated user',
      })
    )
    expect(view.title).toBe('Wrong account')
  })

  it('falls back to a generic wrong-account message when emails are missing', () => {
    const view = mapInvitationError(
      buildApiError(403, { code: 'INVITEE_EMAIL_MISMATCH' })
    )
    expect(view.title).toBe('Wrong account')
    expect(view.description).toContain('different email')
  })

  it('does not misclassify a generic 403 as email mismatch', () => {
    expect(mapInvitationError(buildApiError(403))).toEqual(GENERIC_LOAD_ERROR)
  })

  it('exposes constants for callers to reuse', () => {
    expect(INVALID_LINK_ERROR.title).toBe('Invalid invitation')
    expect(GENERIC_ACCEPT_ERROR.title).toBe("Couldn't accept invitation")
  })
})
