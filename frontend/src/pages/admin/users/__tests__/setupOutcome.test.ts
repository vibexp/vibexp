import { ApiError } from '@/types/errors'

import { setupOutcome, teamRowErrors } from '../setupOutcome'

function validation(fields: string[]) {
  return new ApiError({
    type: 'about:blank',
    title: 'Validation Failed',
    status: 400,
    detail: 'Invalid',
    code: 'VALIDATION_FAILED',
    request_id: 'r',
    timestamp: '2026-10-14T00:00:00Z',
    validation_errors: fields.map(field => ({
      field,
      message: `bad ${field}`,
      code: 'team_invalid',
    })),
  })
}

describe('teamRowErrors', () => {
  it('maps teams[i] refusals to their rows, first message per row', () => {
    expect(
      teamRowErrors(
        validation(['teams[0]', 'teams[2].role', 'teams[0]', 'email'])
      )
    ).toEqual({ 0: 'bad teams[0]', 2: 'bad teams[2].role' })
  })

  it('is empty for anything else', () => {
    expect(teamRowErrors(new Error('boom'))).toEqual({})
    expect(teamRowErrors(validation(['email']))).toEqual({})
  })
})

describe('setupOutcome', () => {
  it('carries the link only for a link delivery', () => {
    expect(
      setupOutcome('a@example.com', {
        delivery: 'link',
        link_url: 'http://x/account/setup#token=t',
        expires_at: '2026-10-21T10:00:00Z',
      }).link
    ).toBe('http://x/account/setup#token=t')
    expect(
      setupOutcome('a@example.com', {
        delivery: 'email',
        expires_at: '2026-10-21T10:00:00Z',
      }).link
    ).toBeNull()
  })

  it('has a fallback for a reason it does not know', () => {
    const outcome = setupOutcome('a@example.com', {
      delivery: 'none',
      reason: 'something_new' as never,
    })
    expect(outcome.tone).toBe('warning')
    expect(outcome.text).toContain('Get one from the user page')
  })
})
