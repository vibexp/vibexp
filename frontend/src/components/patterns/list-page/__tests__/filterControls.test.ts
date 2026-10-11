import { FILTER_ALL, FILTER_LOCKED, lockedParam } from '../filterControls'

describe('lockedParam', () => {
  it('sends locked=true for the "Locked only" value', () => {
    expect(lockedParam(FILTER_LOCKED)).toBe(true)
  })

  it.each([FILTER_ALL, '', 'true', 'false', 'bogus'])(
    'omits the parameter for %j',
    value => {
      // "Unlocked only" is not offered (#1292), so nothing ever sends
      // `locked=false` — and a junk URL value is not a filter either.
      expect(lockedParam(value)).toBeUndefined()
    }
  )
})
