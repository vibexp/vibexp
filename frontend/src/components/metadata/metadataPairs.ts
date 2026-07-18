/**
 * Pure helpers for the {@link MetadataEditor}: they split a resource's
 * `metadata` (`Record<string, unknown>`) into the editable string-valued subset
 * and the preserved non-string "extras", and recombine an edited pair list with
 * those extras back into a single map for the update payload.
 *
 * The editor only ever touches string values; arrays, numbers, booleans, and
 * nested objects live in `extras` and round-trip untouched. Keeping this logic
 * React-free makes the round-trip contract unit-testable in isolation.
 */

/** Maximum number of editable string pairs allowed. */
export const MAX_METADATA_PAIRS = 50
/** Maximum length (trimmed) of a metadata key. */
export const MAX_METADATA_KEY_LENGTH = 128

/** One editable key/value row (both string). */
export interface MetadataPair {
  key: string
  value: string
}

/** The result of splitting a metadata map into editable pairs + preserved extras. */
export interface SplitMetadata {
  /** String-valued entries, in insertion order — the editable rows. */
  pairs: MetadataPair[]
  /** Every non-string entry, preserved verbatim and never shown as a row. */
  extras: Record<string, unknown>
}

/**
 * Partition a metadata map: string values become editable {@link MetadataPair}s
 * (in insertion order); everything else (arrays / nested objects / numbers /
 * booleans / null) is preserved verbatim in `extras`. Mirrors the `extractExtras`
 * pattern already used by the Memory form for its `tags` array.
 */
export function splitMetadata(meta?: Record<string, unknown>): SplitMetadata {
  const pairs: MetadataPair[] = []
  const extras: Record<string, unknown> = {}
  if (!meta) return { pairs, extras }
  for (const [key, value] of Object.entries(meta)) {
    if (typeof value === 'string') {
      pairs.push({ key, value })
    } else {
      extras[key] = value
    }
  }
  return { pairs, extras }
}

/**
 * Recombine edited string pairs with the preserved extras into a single map.
 *
 * Keys are trimmed and blank-keyed pairs are dropped (the editor blocks submit
 * on those, but `onChange` fires on every keystroke, so recombine must tolerate
 * a transient invalid row). `extras` are applied **last** so a preserved
 * non-string value can never be clobbered by a colliding string row — the editor
 * flags that collision as a validation error, but the payload stays correct by
 * construction regardless.
 */
export function recombineMetadata(
  pairs: MetadataPair[],
  extras: Record<string, unknown>
): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const { key, value } of pairs) {
    const trimmedKey = key.trim()
    if (trimmedKey.length === 0) continue
    result[trimmedKey] = value
  }
  return { ...result, ...extras }
}

/** Constraints the host form imposes on the editable rows. */
export interface MetadataValidationOptions {
  /** Keys of preserved non-string extras — a string row may not collide with one. */
  extrasKeys?: string[]
  /** Keys that may not be added as string rows (e.g. Memory's `tags`). */
  reservedKeys?: string[]
  /** Keys whose row may not be deleted nor blanked (e.g. sub-agents `model`). */
  requiredKeys?: string[]
}

/** Per-row validation outcome plus overall validity for the host form. */
export interface MetadataValidationResult {
  /** One entry per input row (same order): an inline message, or `null` if valid. */
  rowErrors: (string | null)[]
  /** A form-level message not tied to a single row (e.g. the pair-count cap). */
  formError: string | null
  /** True when every row is valid and no form-level rule is violated. */
  valid: boolean
}

/**
 * Validate the editable rows against every rule the editor enforces: blank keys,
 * blank values, duplicate keys, over-long keys, reserved keys, collisions with a
 * preserved extra, the pair-count cap, and the "required key may not be blanked"
 * rule. Pure and React-free so every rule is unit-testable in isolation.
 */
export function validateMetadataRows(
  pairs: MetadataPair[],
  options: MetadataValidationOptions = {}
): MetadataValidationResult {
  const extrasKeys = new Set(options.extrasKeys ?? [])
  const reservedKeys = new Set(options.reservedKeys ?? [])
  const requiredKeys = new Set(options.requiredKeys ?? [])

  // Count trimmed non-empty keys so duplicates can be flagged on every offender.
  const keyCounts = new Map<string, number>()
  for (const { key } of pairs) {
    const trimmed = key.trim()
    if (trimmed.length > 0) {
      keyCounts.set(trimmed, (keyCounts.get(trimmed) ?? 0) + 1)
    }
  }

  const rowErrors = pairs.map(({ key, value }) => {
    const trimmedKey = key.trim()
    const isRequired = requiredKeys.has(trimmedKey)
    if (trimmedKey.length === 0) return 'Key is required'
    if (trimmedKey.length > MAX_METADATA_KEY_LENGTH) {
      return `Key must be ${String(MAX_METADATA_KEY_LENGTH)} characters or fewer`
    }
    if (!isRequired && reservedKeys.has(trimmedKey)) {
      return `"${trimmedKey}" is a reserved key`
    }
    if (extrasKeys.has(trimmedKey)) {
      return `"${trimmedKey}" conflicts with an existing non-text value`
    }
    if ((keyCounts.get(trimmedKey) ?? 0) > 1) return 'Duplicate key'
    if (value.trim().length === 0) return 'Value is required'
    return null
  })

  let formError: string | null = null
  if (pairs.length > MAX_METADATA_PAIRS) {
    formError = `At most ${String(MAX_METADATA_PAIRS)} metadata pairs are allowed`
  }

  const valid = formError === null && rowErrors.every(e => e === null)
  return { rowErrors, formError, valid }
}
