import { endOfDay, startOfDay } from 'date-fns'

import {
  advancedKeys,
  ownerEmailParam,
  parseCount,
  parseRange,
  parseTriState,
  sanitizeAdvanced,
  serializeRange,
  serializeTriState,
} from '../advancedFilterParams'

const SPEC = {
  ranges: ['prompt_count'],
  dateRanges: ['last_resource_created'],
  triStates: ['has_projects'],
} as const

describe('advancedKeys', () => {
  it('expands each declaration into its URL keys', () => {
    expect(advancedKeys(SPEC)).toEqual([
      'prompt_count_min',
      'prompt_count_max',
      'last_resource_created_from',
      'last_resource_created_to',
      'has_projects',
    ])
  })

  it('is empty without a spec', () => {
    expect(advancedKeys(undefined)).toEqual([])
    expect(advancedKeys({})).toEqual([])
  })
})

describe('parseCount', () => {
  it.each(['0', '7', '120'])('accepts %s', raw => {
    expect(parseCount(raw)).toBe(Number(raw))
  })

  it.each(['-1', '1.5', 'abc', '01e2', '1e2', '', ' 3', undefined])(
    'rejects %s',
    raw => {
      expect(parseCount(raw)).toBeUndefined()
    }
  )

  it('rejects integers beyond the safe range', () => {
    expect(parseCount('99999999999999999999')).toBeUndefined()
  })
})

describe('parseRange', () => {
  it('keeps valid bounds, either optional', () => {
    expect(parseRange('2', '5')).toEqual({ min: 2, max: 5 })
    expect(parseRange('2', undefined)).toEqual({ min: 2 })
    expect(parseRange(undefined, '5')).toEqual({ max: 5 })
    expect(parseRange('3', '3')).toEqual({ min: 3, max: 3 })
  })

  it('drops both bounds when min > max', () => {
    expect(parseRange('9', '2')).toEqual({})
  })

  it('drops an invalid bound but keeps the valid one', () => {
    expect(parseRange('-1', '4')).toEqual({ max: 4 })
  })
})

describe('parseTriState', () => {
  it('reads true/false and treats anything else as any', () => {
    expect(parseTriState('true')).toBe(true)
    expect(parseTriState('false')).toBe(false)
    expect(parseTriState('yes')).toBeUndefined()
    expect(parseTriState('')).toBeUndefined()
    expect(parseTriState(undefined)).toBeUndefined()
  })
})

describe('serializers', () => {
  it('serializes a range with absent bounds as empty', () => {
    expect(serializeRange('prompt_count', { min: 0 })).toEqual({
      prompt_count_min: '0',
      prompt_count_max: '',
    })
  })

  it('serializes a tri-state, any as empty', () => {
    expect(serializeTriState(true)).toBe('true')
    expect(serializeTriState(false)).toBe('false')
    expect(serializeTriState(undefined)).toBe('')
  })
})

describe('sanitizeAdvanced', () => {
  it('returns typed params and counts a range pair once', () => {
    const { params, activeCount } = sanitizeAdvanced(SPEC, {
      prompt_count_min: '1',
      prompt_count_max: '10',
      has_projects: 'false',
    })
    expect(params).toEqual({
      prompt_count_min: 1,
      prompt_count_max: 10,
      has_projects: false,
    })
    expect(activeCount).toBe(2)
  })

  it('turns date params into local start/end-of-day instants', () => {
    const { params, activeCount } = sanitizeAdvanced(SPEC, {
      last_resource_created_from: '2026-07-01',
      last_resource_created_to: '2026-07-03',
    })
    expect(params).toEqual({
      last_resource_created_from: startOfDay(
        new Date(2026, 6, 1)
      ).toISOString(),
      last_resource_created_to: endOfDay(new Date(2026, 6, 3)).toISOString(),
    })
    expect(activeCount).toBe(1)
  })

  it('never emits invalid values', () => {
    const { params, activeCount } = sanitizeAdvanced(SPEC, {
      prompt_count_min: '8',
      prompt_count_max: '2',
      last_resource_created_from: '2026-02-31',
      has_projects: 'maybe',
    })
    expect(params).toEqual({})
    expect(activeCount).toBe(0)
  })

  it('is empty without a spec', () => {
    expect(sanitizeAdvanced(undefined, { a: '1' })).toEqual({
      params: {},
      activeCount: 0,
    })
  })
})

describe('ownerEmailParam', () => {
  it('trims the address', () => {
    expect(ownerEmailParam(' a@b.co ')).toBe('a@b.co')
  })

  it('refuses anything that is not a bare address', () => {
    // Each of these is refused by the server's mail.ParseAddress round-trip,
    // which answers with a 400.
    for (const value of [
      'boss',
      '@corp.com',
      'boss@',
      'a b@c.d',
      'a@b@c',
      'john..doe@corp.com',
      'a@b..com',
      'john.@corp.com',
      '.john@corp.com',
      'a,b@c.com',
      '<a@b.co>',
      '"a"@b.co',
      'a@[1.2.3.4]',
    ]) {
      expect(ownerEmailParam(value)).toBeUndefined()
    }
  })

  it('accepts ordinary and plus-tagged addresses', () => {
    for (const value of [
      'x@corp.com',
      'first.last+tag@sub.corp.co',
      "o'brien@corp.ie",
      'user_1@a-b.io',
      'jürgen@corp.de',
    ]) {
      expect(ownerEmailParam(value)).toBe(value)
    }
  })

  it('is undefined when blank or absent', () => {
    expect(ownerEmailParam('   ')).toBeUndefined()
    expect(ownerEmailParam(undefined)).toBeUndefined()
  })
})
