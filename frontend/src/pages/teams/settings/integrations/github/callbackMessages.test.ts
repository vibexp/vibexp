import { ApiError } from '@/types/errors'

import {
  describeCallbackFailure,
  MISSING_CODE_MESSAGE,
} from './callbackMessages'

const apiError = (status: number, code: string) =>
  new ApiError({
    status,
    code,
    title: 'title',
    detail: 'detail',
    request_id: 'req-1',
    type: 'about:blank',
  } as ConstructorParameters<typeof ApiError>[0])

describe('describeCallbackFailure', () => {
  // Each arm must tell the admin something DIFFERENT to do — that is the whole
  // point of mapping them. A test that only checked "some string comes back"
  // would pass with every arm collapsed into one message.
  const arms = [
    {
      name: 'org already connected elsewhere',
      error: apiError(409, 'installation_already_connected'),
      expect: /disconnect it there first/i,
    },
    {
      name: 'caller cannot administer the installation',
      error: apiError(403, 'installation_not_authorized'),
      expect: /cannot administer this installation/i,
    },
    {
      name: 'team has no App configured',
      error: apiError(409, 'GITHUB_APP_NOT_CONFIGURED'),
      expect: /Register one under the team’s Settings/i,
    },
    {
      name: 'App has no OAuth credentials',
      error: apiError(503, 'github_user_auth_not_configured'),
      expect: /Client ID and secret/i,
    },
  ] as const

  it.each(arms)('$name gets its own instruction', ({ error, expect: re }) => {
    expect(describeCallbackFailure(error)).toMatch(re)
  })

  it('gives every arm a distinct message', () => {
    const messages = arms.map(a => describeCallbackFailure(a.error))
    expect(new Set(messages).size).toBe(arms.length)
  })

  it('falls back on status for an unrecognised 403', () => {
    // The server may add a code this build has not shipped; a 403 is still an
    // authority problem whatever it is called.
    expect(describeCallbackFailure(apiError(403, 'some_new_code'))).toMatch(
      /not allowed to connect/i
    )
  })

  it('degrades gracefully for a non-API error', () => {
    expect(describeCallbackFailure(new Error('network'))).toMatch(
      /Please try again/i
    )
    expect(describeCallbackFailure(undefined)).toMatch(/Please try again/i)
  })

  it('never leaks server detail into the message', () => {
    // handleError surfaces these verbatim, so anything the server echoed back
    // (paths, ids, upstream text) must not ride along.
    for (const arm of arms) {
      expect(describeCallbackFailure(arm.error)).not.toContain('detail')
    }
  })
})

describe('MISSING_CODE_MESSAGE', () => {
  it('tells the admin to restart the install rather than retry', () => {
    expect(MISSING_CODE_MESSAGE).toMatch(/Start the install again/i)
  })
})
