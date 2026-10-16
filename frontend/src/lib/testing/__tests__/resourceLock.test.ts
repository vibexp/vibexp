import { lockedWriteError, TEST_LOCK } from '../resourceLock'

describe('lockedWriteError', () => {
  it('defaults to the documented locked-field metadata', () => {
    expect(lockedWriteError().metadata).toEqual({
      resource_type: 'artifact',
      field: 'status',
      locked_by: TEST_LOCK.locked_by,
      comment: TEST_LOCK.comment,
    })
  })

  it('gives every error its own copy of the default metadata', () => {
    Object.assign(lockedWriteError().metadata ?? {}, { field: 'title' })

    expect(lockedWriteError().metadata?.field).toBe('status')
  })

  it('uses the metadata a caller passes instead of the default', () => {
    const metadata = { resource_type: 'prompt' }

    expect(lockedWriteError(metadata).metadata).toEqual(metadata)
  })
})
