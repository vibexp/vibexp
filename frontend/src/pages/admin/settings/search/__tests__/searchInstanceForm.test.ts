import type { AdminInstanceSearchSettings } from '@/services/adminSettingsService'

import {
  describeSearchValues,
  type SearchInstanceForm,
  toSearchForm,
  toSearchUpdate,
  validateSearchForm,
} from '../searchInstanceForm'

const values: AdminInstanceSearchSettings['values'] = {
  recency_ranking_enabled: true,
  rank_weight_relevance: 0.5,
  rank_weight_created: 0.3,
  rank_weight_updated: 0.2,
  rank_half_life_days: 90,
  rank_candidate_cap: 200,
}

const limits: AdminInstanceSearchSettings['limits'] = {
  rank_weight_min: 0,
  rank_half_life_days_max: 36500,
  rank_candidate_cap_min: 1,
  rank_candidate_cap_max: 5000,
}

const form = (patch: Partial<SearchInstanceForm> = {}): SearchInstanceForm => ({
  ...toSearchForm(values),
  ...patch,
})

describe('toSearchForm / toSearchUpdate', () => {
  it('round-trips the values', () => {
    expect(toSearchUpdate(toSearchForm(values))).toEqual(values)
  })
})

describe('validateSearchForm', () => {
  it('accepts valid values', () => {
    expect(validateSearchForm(form(), limits)).toEqual({})
  })

  it('rejects a weight below the minimum or not a number', () => {
    expect(
      validateSearchForm(
        form({ rank_weight_created: '-0.1', rank_weight_updated: '' }),
        limits
      )
    ).toEqual({
      rank_weight_created: 'Enter a number of at least 0.',
      rank_weight_updated: 'Enter a number of at least 0.',
    })
  })

  it('flags every weight when all three are zero', () => {
    const errors = validateSearchForm(
      form({
        rank_weight_relevance: '0',
        rank_weight_created: '0',
        rank_weight_updated: '0',
      }),
      limits
    )
    expect(Object.keys(errors)).toEqual([
      'rank_weight_relevance',
      'rank_weight_created',
      'rank_weight_updated',
    ])
    expect(errors.rank_weight_relevance).toBe(
      'The three weights must not all be zero.'
    )
  })

  it.each(['0', '-1', '36501', ''])('rejects a half-life of %j', halfLife => {
    expect(
      validateSearchForm(form({ rank_half_life_days: halfLife }), limits)
    ).toEqual({
      rank_half_life_days: 'Enter a number above 0 and at most 36500.',
    })
  })

  it('accepts a fractional half-life up to the maximum', () => {
    expect(
      validateSearchForm(form({ rank_half_life_days: '0.5' }), limits)
    ).toEqual({})
    expect(
      validateSearchForm(form({ rank_half_life_days: '36500' }), limits)
    ).toEqual({})
  })

  it.each(['0', '5001', '1.5'])('rejects a candidate cap of %j', cap => {
    expect(
      validateSearchForm(form({ rank_candidate_cap: cap }), limits)
    ).toEqual({
      rank_candidate_cap: 'Enter a whole number between 1 and 5000.',
    })
  })

  it('takes the bounds from the limits, not constants', () => {
    expect(
      validateSearchForm(form({ rank_candidate_cap: '300' }), {
        ...limits,
        rank_candidate_cap_max: 250,
      })
    ).toEqual({
      rank_candidate_cap: 'Enter a whole number between 1 and 250.',
    })
  })
})

describe('describeSearchValues', () => {
  it('summarizes every value', () => {
    expect(describeSearchValues(values)).toBe(
      'recency ranking on, weights 0.5 / 0.3 / 0.2 (relevance / created / updated), half-life 90 days, candidate cap 200'
    )
  })
})
