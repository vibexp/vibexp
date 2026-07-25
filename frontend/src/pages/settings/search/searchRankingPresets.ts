import type {
  TeamSearchSettings,
  TeamSearchSettingsValues,
} from '@/services/searchSettingsService'

/**
 * The three ranking profiles offered in the simple view.
 *
 * A preset is nothing but a shorthand for the underlying numbers — there is no
 * "preset" concept in the API or the database, so there is exactly one storage
 * shape and nothing to keep in sync with the backend.
 *
 * `Relevance only` IS `recency_ranking_enabled: false`, which is why the simple
 * view needs no separate on/off switch: the toggle collapses into this radio.
 * Its values are deliberately `null` — when recency ranking is off the weights
 * do not affect ranking at all, so selecting it keeps whatever the team last
 * tuned rather than zeroing their work.
 */
export const RANKING_PRESETS = [
  {
    id: 'relevance-only',
    label: 'Relevance only',
    description:
      'Order purely by how well a resource matches the query. Age is ignored.',
    values: null,
  },
  {
    id: 'balanced',
    label: 'Balanced',
    description:
      'Mostly relevance, with a nudge toward fresher resources. A good default for most teams.',
    values: {
      recency_ranking_enabled: true,
      rank_weight_relevance: 0.5,
      rank_weight_created: 0.3,
      rank_weight_updated: 0.2,
      rank_half_life_days: 90,
    },
  },
  {
    id: 'favor-recent',
    label: 'Favor recent',
    description:
      'Weight freshness heavily and decay faster. Suits fast-moving workspaces where stale context misleads.',
    values: {
      recency_ranking_enabled: true,
      rank_weight_relevance: 0.3,
      rank_weight_created: 0.4,
      rank_weight_updated: 0.3,
      rank_half_life_days: 30,
    },
  },
] as const satisfies readonly {
  id: string
  label: string
  description: string
  values: TeamSearchSettingsValues | null
}[]

export type RankingPresetId = (typeof RANKING_PRESETS)[number]['id'] | 'custom'

// Floating-point tolerance for preset matching. Comparing a round-tripped
// 0.30000000000000004 against 0.3 with === silently falls through to Custom.
const EPSILON = 1e-6

export const closeEnough = (a: number, b: number) => Math.abs(a - b) < EPSILON

/**
 * Derives which preset the given values represent, or `custom` when they match
 * none. The selected radio is always computed from the values — it is never
 * stored, so a profile written straight through the API renders correctly too.
 */
export function detectPreset(
  values: TeamSearchSettingsValues
): RankingPresetId {
  // Recency off is "Relevance only" whatever the (unused) weights say.
  if (!values.recency_ranking_enabled) return 'relevance-only'

  for (const preset of RANKING_PRESETS) {
    const preset_values: TeamSearchSettingsValues | null = preset.values
    if (!preset_values?.recency_ranking_enabled) continue
    if (
      closeEnough(
        preset_values.rank_weight_relevance,
        values.rank_weight_relevance
      ) &&
      closeEnough(
        preset_values.rank_weight_created,
        values.rank_weight_created
      ) &&
      closeEnough(
        preset_values.rank_weight_updated,
        values.rank_weight_updated
      ) &&
      closeEnough(preset_values.rank_half_life_days, values.rank_half_life_days)
    ) {
      return preset.id
    }
  }
  return 'custom'
}

export const sameValues = (
  a: TeamSearchSettingsValues,
  b: TeamSearchSettingsValues
) =>
  a.recency_ranking_enabled === b.recency_ranking_enabled &&
  closeEnough(a.rank_weight_relevance, b.rank_weight_relevance) &&
  closeEnough(a.rank_weight_created, b.rank_weight_created) &&
  closeEnough(a.rank_weight_updated, b.rank_weight_updated) &&
  closeEnough(a.rank_half_life_days, b.rank_half_life_days)

/** The effective profile carried on a settings response, without its context. */
export const effectiveValues = (
  settings: TeamSearchSettings
): TeamSearchSettingsValues => ({
  recency_ranking_enabled: settings.recency_ranking_enabled,
  rank_weight_relevance: settings.rank_weight_relevance,
  rank_weight_created: settings.rank_weight_created,
  rank_weight_updated: settings.rank_weight_updated,
  rank_half_life_days: settings.rank_half_life_days,
})

/** One-line human summary, used to preview what a reset would restore. */
export const describeValues = (values: TeamSearchSettingsValues) =>
  values.recency_ranking_enabled
    ? [
        `relevance ${String(values.rank_weight_relevance)}`,
        `created ${String(values.rank_weight_created)}`,
        `updated ${String(values.rank_weight_updated)}`,
        `half-life ${String(values.rank_half_life_days)}d`,
      ].join(' · ')
    : 'relevance only'

// ---------------------------------------------------------------------------
// Form model
// ---------------------------------------------------------------------------

// The numeric fields are held as strings so a half-typed value ("0.", "") is
// not destroyed on every keystroke; they are parsed for validation, preset
// detection and save.
export interface RankingForm {
  recency_ranking_enabled: boolean
  rank_weight_relevance: string
  rank_weight_created: string
  rank_weight_updated: string
  rank_half_life_days: string
}

export type NumericField = Exclude<keyof RankingForm, 'recency_ranking_enabled'>

export const NUMERIC_FIELDS: {
  key: NumericField
  label: string
  hint: string
  step: string
}[] = [
  {
    key: 'rank_weight_relevance',
    label: 'Relevance weight',
    hint: 'How much semantic match matters. Normally the dominant weight.',
    step: '0.1',
  },
  {
    key: 'rank_weight_created',
    label: 'Created-recency weight',
    hint: 'How much it matters that a resource was created recently.',
    step: '0.1',
  },
  {
    key: 'rank_weight_updated',
    label: 'Updated-recency weight',
    hint: 'How much it matters that a resource was updated recently.',
    step: '0.1',
  },
  {
    key: 'rank_half_life_days',
    label: 'Half-life (days)',
    hint: 'A resource exactly this old scores half its freshness. Shorter decays faster.',
    step: '1',
  },
]

export const toForm = (values: TeamSearchSettingsValues): RankingForm => ({
  recency_ranking_enabled: values.recency_ranking_enabled,
  rank_weight_relevance: String(values.rank_weight_relevance),
  rank_weight_created: String(values.rank_weight_created),
  rank_weight_updated: String(values.rank_weight_updated),
  rank_half_life_days: String(values.rank_half_life_days),
})

export const toValues = (form: RankingForm): TeamSearchSettingsValues => ({
  recency_ranking_enabled: form.recency_ranking_enabled,
  rank_weight_relevance: Number.parseFloat(form.rank_weight_relevance),
  rank_weight_created: Number.parseFloat(form.rank_weight_created),
  rank_weight_updated: Number.parseFloat(form.rank_weight_updated),
  rank_half_life_days: Number.parseFloat(form.rank_half_life_days),
})

// The upper bound the backend enforces on the half-life (config.
// MaxSearchRankHalfLifeDays), mirrored so a typo is caught before the request.
const MAX_HALF_LIFE_DAYS = 36500

/**
 * Mirrors the backend validator (`services.ValidateSearchSettings`) so a tuned
 * profile is rejected here with a readable message instead of as a bare 400.
 */
export function validate(form: RankingForm): string | null {
  const v = toValues(form)
  const weights = [
    v.rank_weight_relevance,
    v.rank_weight_created,
    v.rank_weight_updated,
  ]
  if ([...weights, v.rank_half_life_days].some(n => !Number.isFinite(n))) {
    return 'Every weight and the half-life must be a number.'
  }
  if (weights.some(n => n < 0)) return 'Weights cannot be negative.'
  if (weights.reduce((a, b) => a + b, 0) <= 0) {
    return 'At least one weight must be greater than zero.'
  }
  if (
    v.rank_half_life_days <= 0 ||
    v.rank_half_life_days > MAX_HALF_LIFE_DAYS
  ) {
    return `Half-life must be greater than 0 and at most ${String(MAX_HALF_LIFE_DAYS)} days.`
  }
  return null
}
