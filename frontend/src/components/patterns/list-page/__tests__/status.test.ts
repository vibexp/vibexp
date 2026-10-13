import { isBlankSlate, listPageStatus } from '../status'

describe('listPageStatus', () => {
  it('ranks loading over error over empty over ready', () => {
    expect(listPageStatus(true, 'boom', true)).toBe('loading')
    expect(listPageStatus(false, 'boom', true)).toBe('error')
    expect(listPageStatus(false, null, true)).toBe('empty')
    expect(listPageStatus(false, null, false)).toBe('ready')
  })
})

describe('isBlankSlate', () => {
  it('is an empty list that nothing is filtering', () => {
    expect(isBlankSlate('empty', false)).toBe(true)
  })

  it('is not a list its own filters emptied', () => {
    // That list keeps its filters and offers to clear them.
    expect(isBlankSlate('empty', true)).toBe(false)
  })

  it('is never a list that is loading, failed or holding rows', () => {
    for (const status of ['loading', 'error', 'ready'] as const) {
      expect(isBlankSlate(status, false)).toBe(false)
      expect(isBlankSlate(status, true)).toBe(false)
    }
  })
})
