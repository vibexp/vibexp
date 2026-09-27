import type { TeamAISummarySettingsValues } from '@/services/aiSummarySettingsService'

export type AISummaryStyle = TeamAISummarySettingsValues['style']

export const AI_SUMMARY_STYLES: readonly {
  id: AISummaryStyle
  label: string
}[] = [
  { id: 'concise', label: 'Concise' },
  { id: 'balanced', label: 'Balanced' },
  { id: 'detailed', label: 'Detailed' },
]

/**
 * Form state for the AI Summary card. The numeric knobs are kept as the raw
 * input strings so a user can clear a field mid-edit without it snapping back;
 * `validate` decides whether they are saveable.
 */
export interface AISummaryForm {
  enabled: boolean
  model_provider_id: string | null
  top_n: string
  style: AISummaryStyle
  max_output_tokens: string
}

export function toForm(values: TeamAISummarySettingsValues): AISummaryForm {
  return {
    enabled: values.enabled,
    model_provider_id: values.model_provider_id,
    top_n: String(values.top_n),
    style: values.style,
    max_output_tokens: String(values.max_output_tokens),
  }
}

export function toValues(form: AISummaryForm): TeamAISummarySettingsValues {
  return {
    enabled: form.enabled,
    model_provider_id: form.model_provider_id,
    top_n: Number(form.top_n),
    style: form.style,
    max_output_tokens: Number(form.max_output_tokens),
  }
}

export function sameValues(
  a: TeamAISummarySettingsValues,
  b: TeamAISummarySettingsValues
): boolean {
  return (
    a.enabled === b.enabled &&
    a.model_provider_id === b.model_provider_id &&
    a.top_n === b.top_n &&
    a.style === b.style &&
    a.max_output_tokens === b.max_output_tokens
  )
}

function isPositiveInteger(raw: string): boolean {
  return /^\d+$/.test(raw.trim()) && Number(raw) >= 1
}

/**
 * Keeps a typed integer inside `1..max`. An empty field is left alone so the
 * user can retype; `validate` blocks saving it.
 */
function clampToRange(raw: string, max: number): string {
  if (raw.trim() === '') return raw
  const value = Number(raw)
  if (!Number.isFinite(value)) return raw
  if (value > max) return String(max)
  if (value < 1) return '1'
  return raw
}

/** Keeps a typed `top_n` inside `1..TOP_N_MAX`. */
export function clampTopN(raw: string): string {
  return clampToRange(raw, TOP_N_MAX)
}

/** Keeps a typed `max_output_tokens` inside `1..MAX_OUTPUT_TOKENS_MAX`. */
export function clampMaxOutputTokens(raw: string): string {
  return clampToRange(raw, MAX_OUTPUT_TOKENS_MAX)
}

/**
 * The hard upper bounds of the two numeric knobs, the server's
 * `MaxAISummaryTopN` / `MaxAISummaryOutputTokens`. The settings response
 * still carries the old instance-ceiling fields, but they are deprecated
 * (#1199) and always report these same values, so the form does not read
 * them.
 */
export const TOP_N_MAX = 10
export const MAX_OUTPUT_TOKENS_MAX = 32768

/**
 * Mirrors the server's checks for immediate feedback; the server stays
 * authoritative.
 */
export function validate(form: AISummaryForm): string | null {
  if (!isPositiveInteger(form.top_n) || Number(form.top_n) > TOP_N_MAX) {
    return `Results to read must be a whole number between 1 and ${String(TOP_N_MAX)}.`
  }
  if (
    !isPositiveInteger(form.max_output_tokens) ||
    Number(form.max_output_tokens) > MAX_OUTPUT_TOKENS_MAX
  ) {
    return `Response length must be a whole number between 1 and ${String(MAX_OUTPUT_TOKENS_MAX)}.`
  }
  return null
}

export function describeValues(
  values: TeamAISummarySettingsValues,
  providerName: (id: string | null) => string
): string {
  const style =
    AI_SUMMARY_STYLES.find(s => s.id === values.style)?.label ?? values.style
  return [
    values.enabled ? 'on' : 'off',
    providerName(values.model_provider_id),
    `${String(values.top_n)} results`,
    style.toLowerCase(),
    `${String(values.max_output_tokens)} tokens`,
  ].join(', ')
}
