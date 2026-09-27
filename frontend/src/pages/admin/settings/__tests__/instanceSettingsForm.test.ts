import { ApiError } from '@/types/errors'

import {
  fieldDescribedBy,
  integerRangeError,
  isVersionConflict,
  parseNumber,
  serverFieldErrors,
  VERSION_CONFLICT_CODE,
} from '../instanceSettingsForm'

const apiError = (
  status: number,
  code: string,
  validation_errors?: { field: string; message: string }[]
) =>
  new ApiError({
    type: 'about:blank',
    title: 'Error',
    status,
    detail: 'failed',
    code,
    request_id: 'r1',
    timestamp: '2026-09-27T00:00:00Z',
    validation_errors,
  } as ConstructorParameters<typeof ApiError>[0])

describe('parseNumber', () => {
  it.each([
    ['1.5', 1.5],
    ['0', 0],
    [' 7 ', 7],
  ])('parses %j', (raw, value) => {
    expect(parseNumber(raw)).toBe(value)
  })

  it.each(['', '  ', 'abc', 'Infinity'])('rejects %j', raw => {
    expect(parseNumber(raw)).toBeNull()
  })
})

describe('integerRangeError', () => {
  it('accepts whole numbers inside the bounds, inclusive', () => {
    expect(integerRangeError('1', 1, 10)).toBeNull()
    expect(integerRangeError('10', 1, 10)).toBeNull()
  })

  it.each(['0', '11', '2.5', '', 'x'])('rejects %j', raw => {
    expect(integerRangeError(raw, 1, 10)).toBe(
      'Enter a whole number between 1 and 10.'
    )
  })
})

describe('isVersionConflict', () => {
  it('is true only for the settings version-conflict 409', () => {
    expect(isVersionConflict(apiError(409, VERSION_CONFLICT_CODE))).toBe(true)
    expect(isVersionConflict(apiError(409, 'OTHER'))).toBe(false)
    expect(isVersionConflict(apiError(400, VERSION_CONFLICT_CODE))).toBe(false)
    expect(isVersionConflict(new Error('x'))).toBe(false)
  })
})

describe('serverFieldErrors', () => {
  const fields = ['top_n', 'style'] as const

  it('places known fields and collects the rest', () => {
    expect(
      serverFieldErrors(
        apiError(400, 'INSTANCE_SETTINGS_VALIDATION_FAILED', [
          { field: 'top_n', message: 'too big' },
          { field: 'top_n', message: 'second' },
          { field: 'body', message: 'malformed' },
        ]),
        fields
      )
    ).toEqual({ fields: { top_n: 'too big' }, other: ['malformed'] })
  })

  it('is null for anything but a 400 with field errors', () => {
    expect(serverFieldErrors(apiError(400, 'X'), fields)).toBeNull()
    expect(serverFieldErrors(apiError(500, 'X'), fields)).toBeNull()
    expect(serverFieldErrors(new Error('x'), fields)).toBeNull()
  })
})

describe('fieldDescribedBy', () => {
  it('names the hint and the error when present', () => {
    expect(fieldDescribedBy('f', 'hint', 'err')).toBe('f-hint f-error')
    expect(fieldDescribedBy('f', 'hint', undefined)).toBe('f-hint')
    expect(fieldDescribedBy('f', undefined, undefined)).toBeUndefined()
  })
})
