import { ApiError } from '@/types/errors'

const makeError = (code: string, status: number) =>
  new ApiError({
    type: `https://api.vibexp.io/errors/${code}`,
    title: code,
    status,
    detail: 'detail',
    code,
    request_id: 'req-1',
    timestamp: new Date().toISOString(),
  })

describe('ApiError.isForbidden (#225)', () => {
  it('identifies the backend FORBIDDEN code', () => {
    expect(makeError('FORBIDDEN', 403).isForbidden()).toBe(true)
  })

  it('does not treat other errors as forbidden', () => {
    expect(makeError('RESOURCE_NOT_FOUND', 404).isForbidden()).toBe(false)
    expect(makeError('VALIDATION_FAILED', 400).isForbidden()).toBe(false)
    expect(makeError('RESOURCE_LIMIT_EXCEEDED', 429).isForbidden()).toBe(false)
  })

  it('is disjoint from isAuthError, so a 403 never triggers the logout redirect', () => {
    const forbidden = makeError('FORBIDDEN', 403)

    expect(forbidden.isAuthError()).toBe(false)

    // ...and the converse: an expired session is not a permission problem.
    const expired = makeError('AUTH_EXPIRED', 401)
    expect(expired.isForbidden()).toBe(false)
    expect(expired.isAuthError()).toBe(true)
  })
})
