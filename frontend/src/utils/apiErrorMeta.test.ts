import type { APIErrorResponse } from '@/types/errors'
import { ApiError } from '@/types/errors'

import { readStringMeta } from './apiErrorMeta'

const buildApiError = (overrides: Partial<APIErrorResponse> = {}): ApiError =>
  new ApiError({
    type: 'https://api.vibexp.io/errors/test',
    title: 'Test Error',
    status: 409,
    detail: 'detail',
    code: 'TEST',
    request_id: 'req-1',
    timestamp: '2024-01-01T00:00:00Z',
    ...overrides,
  })

describe('readStringMeta', () => {
  it('returns the value for a present non-empty string key', () => {
    const error = buildApiError({ metadata: { foo: 'bar' } })
    expect(readStringMeta(error, 'foo')).toBe('bar')
  })

  it('returns the first non-empty string among multiple keys', () => {
    const error = buildApiError({ metadata: { second: 'value' } })
    expect(readStringMeta(error, 'first', 'second')).toBe('value')
  })

  it('returns undefined when metadata is absent', () => {
    const error = buildApiError()
    expect(readStringMeta(error, 'foo')).toBeUndefined()
  })

  it('returns undefined when the key is missing', () => {
    const error = buildApiError({ metadata: { other: 'x' } })
    expect(readStringMeta(error, 'foo')).toBeUndefined()
  })

  it('returns undefined for empty-string and non-string values', () => {
    const error = buildApiError({
      metadata: { empty: '', num: 42, obj: { nested: true } },
    })
    expect(readStringMeta(error, 'empty')).toBeUndefined()
    expect(readStringMeta(error, 'num')).toBeUndefined()
    expect(readStringMeta(error, 'obj')).toBeUndefined()
  })

  it('ignores inherited prototype properties', () => {
    const error = buildApiError({ metadata: {} })
    expect(readStringMeta(error, 'toString')).toBeUndefined()
  })
})
