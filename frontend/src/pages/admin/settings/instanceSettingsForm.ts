import { ApiError } from '@/types/errors'

/**
 * Form plumbing shared by the instance search and AI summary settings pages
 * (#1202). A data module rather than helpers in a `.tsx` for the same reason
 * as `emailProviderForm.ts` (#587).
 *
 * Numeric inputs are kept as the raw strings the admin typed, so a field can
 * be cleared mid-edit without snapping back; the validators decide whether a
 * value is saveable. Form keys are the wire field names, so a server field
 * error lands on its input without a mapping table.
 */

/** Per-field messages, keyed by the form (= wire) field name. */
export type FieldErrors<K extends string = string> = Partial<Record<K, string>>

/** The error code of a save that lost an optimistic-lock race. */
export const VERSION_CONFLICT_CODE = 'INSTANCE_SETTINGS_VERSION_CONFLICT'

/** A finite number parsed from an input, or null when it is not one. */
export function parseNumber(raw: string): number | null {
  if (raw.trim() === '') return null
  const value = Number(raw)
  return Number.isFinite(value) ? value : null
}

/**
 * The message for a whole number outside `min..max`, or null when it is in
 * range. The bounds come from the response's `limits`, never hardcoded.
 */
export function integerRangeError(
  raw: string,
  min: number,
  max: number
): string | null {
  const value = parseNumber(raw)
  if (
    value === null ||
    !Number.isInteger(value) ||
    value < min ||
    value > max
  ) {
    return `Enter a whole number between ${String(min)} and ${String(max)}.`
  }
  return null
}

/** Whether `err` is the 409 of a save over someone else's change. */
export function isVersionConflict(err: unknown): boolean {
  return (
    err instanceof ApiError &&
    err.status === 409 &&
    err.code === VERSION_CONFLICT_CODE
  )
}

/**
 * Splits a 400's `validation_errors` into the ones that name a field of this
 * form (shown inline) and the rest (shown at form level). A rule spanning
 * several fields (e.g. "the weights must not all be zero") names each one, so
 * each gets the message. Not a 400 with field errors → null.
 */
export function serverFieldErrors<K extends string>(
  err: unknown,
  fields: readonly K[]
): { fields: FieldErrors<K>; other: string[] } | null {
  if (!(err instanceof ApiError) || err.status !== 400) return null
  const errors = err.validationErrors ?? []
  if (errors.length === 0) return null
  const placed: FieldErrors<K> = {}
  const other: string[] = []
  for (const { field, message } of errors) {
    if ((fields as readonly string[]).includes(field)) {
      placed[field as K] ??= message
    } else {
      other.push(message)
    }
  }
  return { fields: placed, other }
}

/**
 * `aria-describedby` for a settings field: its hint (`<id>-hint`) and, when
 * present, its error (`<id>-error`).
 */
export function fieldDescribedBy(
  id: string,
  hint: unknown,
  error: unknown
): string | undefined {
  const ids = [hint ? `${id}-hint` : null, error ? `${id}-error` : null]
  return ids.filter(Boolean).join(' ') || undefined
}
