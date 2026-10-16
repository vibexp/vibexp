import type { AdminInstanceSettingsAuditPage } from '@/services/adminService'
import type {
  AdminAuditLogSettings,
  AdminAuditLogSettingsUpdate,
} from '@/services/adminSettingsService'

import { type AuditField, formatAuditValue } from '../instanceSettingsAudit'
import {
  type FieldErrors,
  integerRangeError,
  parseNumber,
} from '../instanceSettingsForm'
import type { InstanceSettingsEnvelope } from '../useInstanceSettingsEditor'

/**
 * Form state, validation and wire mapping for Admin → Settings → Audit log
 * (#1518). The retention bounds come from the response's `limits`; the server
 * stays authoritative.
 */

type AuditLogValues = AdminAuditLogSettings['values']
type AuditLogLimits = AdminAuditLogSettings['limits']

export interface AuditLogSettingsForm {
  retention_weeks: string
  capture_ip_ua: boolean
}

export type AuditLogFormField = keyof AuditLogSettingsForm

export const AUDIT_LOG_FORM_FIELDS: readonly AuditLogFormField[] = [
  'retention_weeks',
  'capture_ip_ua',
]

/** The history's allowlist, in form order. */
export const AUDIT_LOG_AUDIT_FIELDS: readonly AuditField[] = [
  ['retention_weeks', 'Retention (weeks)'],
  ['capture_ip_ua', 'Capture IP address and user agent'],
]

/** What the shared instance-settings editor needs around the values. */
export type AuditLogEnvelope = InstanceSettingsEnvelope<
  AuditLogValues,
  AuditLogLimits
>

/**
 * Teams cannot override the audit-log settings, so the response carries no
 * override count; the shared editor expects one, so it is filled in here.
 */
export function toAuditLogEnvelope(
  response: AdminAuditLogSettings
): AuditLogEnvelope {
  return {
    values: response.values,
    built_in_defaults: response.built_in_defaults,
    limits: response.limits,
    source: response.source,
    teams_with_override: 0,
    updated_at: response.updated_at,
    updated_by_name: response.updated_by_name,
    version: response.version,
  }
}

export function toAuditLogForm(values: AuditLogValues): AuditLogSettingsForm {
  return {
    retention_weeks: String(values.retention_weeks),
    capture_ip_ua: values.capture_ip_ua,
  }
}

/** The PUT body without `expected_version`; call only on a valid form. */
export function toAuditLogUpdate(
  form: AuditLogSettingsForm
): Omit<AdminAuditLogSettingsUpdate, 'expected_version'> {
  return {
    retention_weeks: Number(form.retention_weeks),
    capture_ip_ua: form.capture_ip_ua,
  }
}

export function validateAuditLogForm(
  form: AuditLogSettingsForm,
  limits: AuditLogLimits
): FieldErrors<AuditLogFormField> {
  const errors: FieldErrors<AuditLogFormField> = {}
  const retentionError = integerRangeError(
    form.retention_weeks,
    limits.retention_weeks_min,
    limits.retention_weeks_max
  )
  if (retentionError) errors.retention_weeks = retentionError
  return errors
}

/**
 * The warning shown while the form's retention is below the SAVED one (not
 * the last keystroke): the next daily purge deletes what falls outside it.
 * Null when retention is not lowered or the field is not a valid number.
 */
export function retentionLoweredWarning(
  form: AuditLogSettingsForm,
  saved: AuditLogValues
): string | null {
  const weeks = parseNumber(form.retention_weeks)
  if (weeks === null || !Number.isInteger(weeks) || weeks < 1) return null
  if (weeks >= saved.retention_weeks) return null
  return `${purgeSentence(weeks)} This cannot be undone.`
}

function purgeSentence(weeks: number): string {
  const unit = weeks === 1 ? 'week' : 'weeks'
  return `Audit events older than ${String(weeks)} ${unit} will be deleted at the next daily purge.`
}

/** A one-line summary of a set of values, for the reset preview. */
export function describeAuditLogValues(values: AuditLogValues): string {
  const unit = values.retention_weeks === 1 ? 'week' : 'weeks'
  return [
    `retention ${String(values.retention_weeks)} ${unit}`,
    `IP address and user agent capture ${values.capture_ip_ua ? 'on' : 'off'}`,
  ].join(', ')
}

/**
 * The reset preview: the built-in defaults, plus the purge a reset triggers
 * when they keep events for less time than the saved settings do. The layout
 * appends the final period.
 */
export function describeAuditLogReset(
  defaults: AuditLogValues,
  saved: AuditLogValues
): string {
  const summary = describeAuditLogValues(defaults)
  if (defaults.retention_weeks >= saved.retention_weeks) return summary
  return `${summary}. ${purgeSentence(defaults.retention_weeks).slice(0, -1)}`
}

type AuditSnapshot = Record<string, unknown>

/**
 * The built-in defaults standing in for a missing history snapshot. A null
 * `before` (first save) or `after` (reset) means nothing was stored, so the
 * built-in defaults applied: the history shows them as "2 (default)" rather
 * than "—". A field whose default equals the other side keeps its raw value,
 * so it compares equal and drops out of the diff.
 */
function defaultSnapshot(
  defaults: AuditLogValues,
  other: AuditSnapshot
): AuditSnapshot {
  const current = new Map(Object.entries(other))
  return Object.fromEntries(
    Object.entries(defaults).map(([key, value]) => [
      key,
      current.get(key) === value
        ? value
        : `${formatAuditValue(value)} (default)`,
    ])
  )
}

/**
 * The history page with missing snapshots filled from the built-in defaults;
 * unchanged while the defaults are not known yet.
 */
export function withDefaultSnapshots(
  page: AdminInstanceSettingsAuditPage,
  defaults: AuditLogValues | undefined
): AdminInstanceSettingsAuditPage {
  if (!defaults) return page
  return {
    ...page,
    entries: page.entries.map(entry => {
      if (entry.before === null && entry.after !== null) {
        return { ...entry, before: defaultSnapshot(defaults, entry.after) }
      }
      if (entry.after === null && entry.before !== null) {
        return { ...entry, after: defaultSnapshot(defaults, entry.before) }
      }
      return entry
    }),
  }
}
