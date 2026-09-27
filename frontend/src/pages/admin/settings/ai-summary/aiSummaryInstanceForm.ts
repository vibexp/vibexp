import type {
  AdminInstanceAISummarySettings,
  AdminInstanceAISummarySettingsUpdate,
} from '@/services/adminSettingsService'

import type { AuditField } from '../instanceSettingsAudit'
import {
  type FieldErrors,
  integerRangeError,
  parseNumber,
} from '../instanceSettingsForm'

/**
 * Form state, validation and wire mapping for Admin → Settings → AI Summary
 * (#1202). The rules mirror the server's (`validateAISummaryProfileBounds`
 * plus the instance budget checks) for immediate feedback; the server stays
 * authoritative.
 */

type AISummaryValues = AdminInstanceAISummarySettings['values']
type AISummaryLimits = AdminInstanceAISummarySettings['limits']
export type AISummaryStyle = AISummaryValues['style']

export interface AISummaryInstanceForm {
  enabled: boolean
  top_n: string
  style: AISummaryStyle
  max_output_tokens: string
  per_document_chars: string
  total_context_chars: string
  request_timeout_ms: string
}

export type AISummaryFormField = keyof AISummaryInstanceForm

export const AI_SUMMARY_FORM_FIELDS: readonly AISummaryFormField[] = [
  'enabled',
  'top_n',
  'style',
  'max_output_tokens',
  'per_document_chars',
  'total_context_chars',
  'request_timeout_ms',
]

/** The history's allowlist, in form order. */
export const AI_SUMMARY_AUDIT_FIELDS: readonly AuditField[] = [
  ['enabled', 'Enabled'],
  ['top_n', 'Results to read'],
  ['style', 'Style'],
  ['max_output_tokens', 'Response length (tokens)'],
  ['per_document_chars', 'Per-document budget (chars)'],
  ['total_context_chars', 'Total context budget (chars)'],
  ['request_timeout_ms', 'Request timeout (ms)'],
]

export function toAISummaryForm(
  values: AISummaryValues
): AISummaryInstanceForm {
  return {
    enabled: values.enabled,
    top_n: String(values.top_n),
    style: values.style,
    max_output_tokens: String(values.max_output_tokens),
    per_document_chars: String(values.per_document_chars),
    total_context_chars: String(values.total_context_chars),
    request_timeout_ms: String(values.request_timeout_ms),
  }
}

/** The PUT body without `expected_version`; call only on a valid form. */
export function toAISummaryUpdate(
  form: AISummaryInstanceForm
): Omit<AdminInstanceAISummarySettingsUpdate, 'expected_version'> {
  return {
    enabled: form.enabled,
    top_n: Number(form.top_n),
    style: form.style,
    max_output_tokens: Number(form.max_output_tokens),
    per_document_chars: Number(form.per_document_chars),
    total_context_chars: Number(form.total_context_chars),
    request_timeout_ms: Number(form.request_timeout_ms),
  }
}

export function validateAISummaryForm(
  form: AISummaryInstanceForm,
  limits: AISummaryLimits
): FieldErrors<AISummaryFormField> {
  const errors: FieldErrors<AISummaryFormField> = {}
  const check = (
    field: AISummaryFormField,
    raw: string,
    min: number,
    max: number
  ) => {
    const message = integerRangeError(raw, min, max)
    if (message) errors[field] = message
  }

  check('top_n', form.top_n, limits.top_n_min, limits.top_n_max)
  check(
    'max_output_tokens',
    form.max_output_tokens,
    limits.max_output_tokens_min,
    limits.max_output_tokens_max
  )
  check(
    'per_document_chars',
    form.per_document_chars,
    limits.chars_min,
    limits.chars_max
  )
  check(
    'total_context_chars',
    form.total_context_chars,
    limits.chars_min,
    limits.chars_max
  )
  check(
    'request_timeout_ms',
    form.request_timeout_ms,
    limits.request_timeout_ms_min,
    limits.request_timeout_ms_max
  )

  const perDocument = parseNumber(form.per_document_chars)
  const total = parseNumber(form.total_context_chars)
  if (
    !errors.per_document_chars &&
    !errors.total_context_chars &&
    perDocument !== null &&
    total !== null &&
    total < perDocument
  ) {
    errors.total_context_chars =
      'Must be at least the per-document budget, so one document fits.'
  }

  return errors
}

/** A one-line summary of a set of values, for the reset preview. */
export function describeAISummaryValues(values: AISummaryValues): string {
  return [
    values.enabled ? 'enabled' : 'disabled',
    `${String(values.top_n)} results`,
    values.style,
    `${String(values.max_output_tokens)} tokens`,
    `${String(values.per_document_chars)} chars per document`,
    `${String(values.total_context_chars)} chars in total`,
    `${String(values.request_timeout_ms)} ms timeout`,
  ].join(', ')
}
