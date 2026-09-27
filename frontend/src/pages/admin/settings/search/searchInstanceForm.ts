import type {
  AdminInstanceSearchSettings,
  AdminInstanceSearchSettingsUpdate,
} from '@/services/adminSettingsService'

import type { AuditField } from '../instanceSettingsAudit'
import {
  type FieldErrors,
  integerRangeError,
  parseNumber,
} from '../instanceSettingsForm'

/**
 * Form state, validation and wire mapping for Admin → Settings → Search
 * (#1202). The rules mirror the server's (`validateSearchRankingWeightsAndHalfLife` and the candidate-cap bounds) for
 * immediate feedback; the server stays authoritative.
 */

type SearchValues = AdminInstanceSearchSettings['values']
type SearchLimits = AdminInstanceSearchSettings['limits']

export const WEIGHT_FIELDS = [
  'rank_weight_relevance',
  'rank_weight_created',
  'rank_weight_updated',
] as const

export type WeightField = (typeof WEIGHT_FIELDS)[number]

export interface SearchInstanceForm {
  recency_ranking_enabled: boolean
  rank_weight_relevance: string
  rank_weight_created: string
  rank_weight_updated: string
  rank_half_life_days: string
  rank_candidate_cap: string
}

export type SearchFormField = keyof SearchInstanceForm

export const SEARCH_FORM_FIELDS: readonly SearchFormField[] = [
  'recency_ranking_enabled',
  ...WEIGHT_FIELDS,
  'rank_half_life_days',
  'rank_candidate_cap',
]

export const WEIGHT_LABELS: Record<WeightField, string> = {
  rank_weight_relevance: 'Relevance weight',
  rank_weight_created: 'Created-recency weight',
  rank_weight_updated: 'Updated-recency weight',
}

/** The history's allowlist, in form order. */
export const SEARCH_AUDIT_FIELDS: readonly AuditField[] = [
  ['recency_ranking_enabled', 'Recency ranking'],
  ['rank_weight_relevance', WEIGHT_LABELS.rank_weight_relevance],
  ['rank_weight_created', WEIGHT_LABELS.rank_weight_created],
  ['rank_weight_updated', WEIGHT_LABELS.rank_weight_updated],
  ['rank_half_life_days', 'Half-life (days)'],
  ['rank_candidate_cap', 'Candidate cap'],
]

export function toSearchForm(values: SearchValues): SearchInstanceForm {
  return {
    recency_ranking_enabled: values.recency_ranking_enabled,
    rank_weight_relevance: String(values.rank_weight_relevance),
    rank_weight_created: String(values.rank_weight_created),
    rank_weight_updated: String(values.rank_weight_updated),
    rank_half_life_days: String(values.rank_half_life_days),
    rank_candidate_cap: String(values.rank_candidate_cap),
  }
}

/** The PUT body without `expected_version`; call only on a valid form. */
export function toSearchUpdate(
  form: SearchInstanceForm
): Omit<AdminInstanceSearchSettingsUpdate, 'expected_version'> {
  return {
    recency_ranking_enabled: form.recency_ranking_enabled,
    rank_weight_relevance: Number(form.rank_weight_relevance),
    rank_weight_created: Number(form.rank_weight_created),
    rank_weight_updated: Number(form.rank_weight_updated),
    rank_half_life_days: Number(form.rank_half_life_days),
    rank_candidate_cap: Number(form.rank_candidate_cap),
  }
}

export function validateSearchForm(
  form: SearchInstanceForm,
  limits: SearchLimits
): FieldErrors<SearchFormField> {
  const errors: FieldErrors<SearchFormField> = {}

  const weights = WEIGHT_FIELDS.map(field => parseNumber(form[field]))
  WEIGHT_FIELDS.forEach((field, i) => {
    const weight = weights[i]
    if (weight === null || weight < limits.rank_weight_min) {
      errors[field] =
        `Enter a number of at least ${String(limits.rank_weight_min)}.`
    }
  })
  if (weights.every(weight => weight === 0)) {
    for (const field of WEIGHT_FIELDS) {
      errors[field] = 'The three weights must not all be zero.'
    }
  }

  const halfLife = parseNumber(form.rank_half_life_days)
  if (
    halfLife === null ||
    halfLife <= 0 ||
    halfLife > limits.rank_half_life_days_max
  ) {
    errors.rank_half_life_days = `Enter a number above 0 and at most ${String(limits.rank_half_life_days_max)}.`
  }

  const capError = integerRangeError(
    form.rank_candidate_cap,
    limits.rank_candidate_cap_min,
    limits.rank_candidate_cap_max
  )
  if (capError) errors.rank_candidate_cap = capError

  return errors
}

/** A one-line summary of a set of values, for the reset preview. */
export function describeSearchValues(values: SearchValues): string {
  return [
    `recency ranking ${values.recency_ranking_enabled ? 'on' : 'off'}`,
    `weights ${String(values.rank_weight_relevance)} / ${String(values.rank_weight_created)} / ${String(values.rank_weight_updated)} (relevance / created / updated)`,
    `half-life ${String(values.rank_half_life_days)} days`,
    `candidate cap ${String(values.rank_candidate_cap)}`,
  ].join(', ')
}
