// @vibexp/api-client ships ESM only; stub it out for the
// jest/CJS test environment (same approach as the firebase mocks). Tests
// exercising migrated services should mock `@/lib/apiClientGenerated`
// instead of letting calls reach this stub.

const reject = (method: string) => () =>
  Promise.reject(
    new Error(
      `vibexp-api-client stub: ${method} called — mock @/lib/apiClientGenerated in tests`
    )
  )

export const PRODUCTION_BASE_URL = 'https://api.vibexp.io'

export const createApiClient = jest.fn(() => ({
  GET: reject('GET'),
  POST: reject('POST'),
  PUT: reject('PUT'),
  PATCH: reject('PATCH'),
  DELETE: reject('DELETE'),
  HEAD: reject('HEAD'),
  OPTIONS: reject('OPTIONS'),
  TRACE: reject('TRACE'),
}))

export default createApiClient
