import { lockedWriteError, TEST_LOCK } from '@/lib/testing/resourceLock'
import { ApiError } from '@/types/errors'

import { isLockedWrite, lockedWriteMessage } from '../lockedWrite'

const documented = {
  resource_type: 'artifact',
  locked_by: TEST_LOCK.locked_by,
  comment: TEST_LOCK.comment,
}

function apiError(status: number, code: string) {
  return new ApiError({
    type: `https://api.vibexp.io/errors/${code}`,
    title: 'Locked',
    status,
    detail: 'project has locked resources',
    code,
    request_id: 'req-1',
    timestamp: '2026-10-11T10:16:00Z',
  })
}

describe('isLockedWrite', () => {
  it('recognises the 423 a lock refuses a write with', () => {
    expect(isLockedWrite(lockedWriteError())).toBe(true)
  })

  it.each([
    // The other 423: a project delete blocked by locked resources (#1290).
    ['another 423 code', apiError(423, 'HAS_LOCKED_RESOURCES')],
    ['the code on another status', apiError(409, 'RESOURCE_WRITE_LOCKED')],
    ['a plain Error', new Error('RESOURCE_WRITE_LOCKED')],
    ['a non-error', 'RESOURCE_WRITE_LOCKED'],
  ])('rejects %s', (_name, error) => {
    expect(isLockedWrite(error)).toBe(false)
    expect(lockedWriteMessage(error)).toBeNull()
  })
})

describe('lockedWriteMessage', () => {
  it('names the field, the locker and the comment for a refused update', () => {
    expect(
      lockedWriteMessage(lockedWriteError({ ...documented, field: 'status' }))
    ).toBe(
      'The status of this artifact cannot be changed: it is locked by Ada Lovelace ("Used by the nightly release agent"). Unlock it first.'
    )
  })

  it.each([
    ['project_id', 'The project of this prompt'],
    ['mcp_expose', 'The MCP exposure of this prompt'],
    ['some_new_field', 'The some new field of this prompt'],
  ])('reads the request field %s as words', (field, start) => {
    const message = lockedWriteMessage(
      lockedWriteError({ ...documented, resource_type: 'prompt', field })
    )
    expect(message).toContain(`${start} cannot be changed`)
  })

  it('says the resource cannot be deleted when the server names no field', () => {
    expect(lockedWriteMessage(lockedWriteError(documented))).toBe(
      'This artifact is locked by Ada Lovelace ("Used by the nightly release agent") and cannot be deleted until it is unlocked.'
    )
  })

  it('leaves the comment out when the lock has none', () => {
    expect(
      lockedWriteMessage(lockedWriteError({ ...documented, comment: '  ' }))
    ).toBe(
      'This artifact is locked by Ada Lovelace and cannot be deleted until it is unlocked.'
    )
  })

  it.each([
    ['no locker', { resource_type: 'artifact', comment: 'x' }],
    ['a locker that is not an object', { ...documented, locked_by: 'Ada' }],
    ['a locker with no name', { ...documented, locked_by: { user_id: 'u' } }],
    ['no resource type', { locked_by: TEST_LOCK.locked_by }],
    ['a null locker', { ...documented, locked_by: null }],
  ])('falls back to the server message for metadata with %s', (_n, meta) => {
    expect(lockedWriteMessage(lockedWriteError(meta))).toBe(
      'artifact is locked'
    )
  })

  it('falls back to the server message when there is no metadata at all', () => {
    const error = new ApiError({ ...lockedWriteError().response })
    Object.defineProperty(error, 'metadata', { value: undefined })
    expect(lockedWriteMessage(error)).toBe('artifact is locked')
  })
})
