import type { DateRangeValue } from '@/components/ui/date-range'
import { fromDateParam, rangeToInstants } from '@/components/ui/date-range'

/**
 * Pure parse/validate/serialize helpers behind the admin "Advanced filters"
 * panel (#1132).
 *
 * Shared by `useAdminListFilters` (which cleans what arrives in the URL) and the
 * controls (which refuse to commit invalid input), so both sides agree on what a
 * valid value is. The per-page wiring (#1134/#1139/#1144), saved presets (#1148)
 * and CSV export (#1150) all read the cleaned `params` this module produces
 * rather than re-deriving them from raw URL strings.
 */

/**
 * The advanced filters a page declares, by base name.
 *
 * - `ranges` expand to `${name}_min` / `${name}_max` (inclusive, integers ≥ 0)
 * - `dateRanges` expand to `${name}_from` / `${name}_to` (local `YYYY-MM-DD` days)
 * - `triStates` use `${name}` itself (`true` / `false`, absent = any)
 *
 * The naming matches the admin list API (#1133), e.g. `prompt_count_min` and
 * `last_resource_created_from`.
 */
export interface AdvancedFilterSpec {
  ranges?: readonly string[]
  dateRanges?: readonly string[]
  triStates?: readonly string[]
}

/** A numeric range; either bound may be absent. */
export interface NumberRangeValue {
  min?: number
  max?: number
}

export type AdvancedParamValue = number | boolean | string

export const rangeKeys = (name: string) =>
  [`${name}_min`, `${name}_max`] as const

export const dateRangeKeys = (name: string) =>
  [`${name}_from`, `${name}_to`] as const

/** Every URL key a spec owns, flattened. */
export function advancedKeys(spec: AdvancedFilterSpec | undefined): string[] {
  if (!spec) return []
  return [
    ...(spec.ranges ?? []).flatMap(name => [...rangeKeys(name)]),
    ...(spec.dateRanges ?? []).flatMap(name => [...dateRangeKeys(name)]),
    ...(spec.triStates ?? []),
  ]
}

/**
 * A count bound: digits only, so `-1`, `1.5`, `abc` and `1e2` are rejected
 * rather than coerced the way `Number()` would. `undefined` when absent or
 * invalid.
 */
export function parseCount(raw: string | undefined): number | undefined {
  if (raw === undefined || !/^\d+$/.test(raw)) return undefined
  const value = Number(raw)
  return Number.isSafeInteger(value) ? value : undefined
}

/**
 * Both bounds of a range, or `{}` when they contradict each other (min > max).
 * Sending half of a contradictory pair would silently change what the admin
 * meant, so the pair is dropped whole.
 */
export function parseRange(
  rawMin: string | undefined,
  rawMax: string | undefined
): NumberRangeValue {
  const min = parseCount(rawMin)
  const max = parseCount(rawMax)
  if (min !== undefined && max !== undefined && min > max) return {}
  return {
    ...(min === undefined ? {} : { min }),
    ...(max === undefined ? {} : { max }),
  }
}

/** `true` / `false`; anything else (including absent) is "any". */
export function parseTriState(raw: string | undefined): boolean | undefined {
  if (raw === 'true') return true
  if (raw === 'false') return false
  return undefined
}

/** A date range from its URL params; malformed days are dropped. */
export function parseDateRange(
  rawFrom: string | undefined,
  rawTo: string | undefined
): DateRangeValue {
  return { from: fromDateParam(rawFrom), to: fromDateParam(rawTo) }
}

/** Serializes a range for the URL; an absent bound is `''` (removed). */
export function serializeRange(
  name: string,
  value: NumberRangeValue
): Record<string, string> {
  const [minKey, maxKey] = rangeKeys(name)
  return {
    [minKey]: value.min === undefined ? '' : String(value.min),
    [maxKey]: value.max === undefined ? '' : String(value.max),
  }
}

/** Serializes a tri-state for the URL; "any" is `''` (removed). */
export function serializeTriState(value: boolean | undefined): string {
  return value === undefined ? '' : String(value)
}

/**
 * The cleaned, typed request fragment for every declared advanced filter, and
 * how many filters are active (a range pair counts once, however many of its
 * bounds are set). Invalid values never appear in `params`.
 */
export function sanitizeAdvanced(
  spec: AdvancedFilterSpec | undefined,
  filters: Readonly<Record<string, string | undefined>>
): { params: Record<string, AdvancedParamValue>; activeCount: number } {
  const params: Record<string, AdvancedParamValue> = {}
  let activeCount = 0
  if (!spec) return { params, activeCount }

  for (const name of spec.ranges ?? []) {
    const [minKey, maxKey] = rangeKeys(name)
    const { min, max } = parseRange(filters[minKey], filters[maxKey])
    if (min !== undefined) params[minKey] = min
    if (max !== undefined) params[maxKey] = max
    if (min !== undefined || max !== undefined) activeCount += 1
  }

  for (const name of spec.dateRanges ?? []) {
    const [fromKey, toKey] = dateRangeKeys(name)
    const { from, to } = rangeToInstants(
      parseDateRange(filters[fromKey], filters[toKey])
    )
    if (from !== undefined) params[fromKey] = from
    if (to !== undefined) params[toKey] = to
    if (from !== undefined || to !== undefined) activeCount += 1
  }

  for (const name of spec.triStates ?? []) {
    const value = parseTriState(filters[name])
    if (value !== undefined) {
      params[name] = value
      activeCount += 1
    }
  }

  return { params, activeCount }
}

/**
 * A bare RFC 5322 dot-atom address: the shape the server's check
 * (`mail.ParseAddress` round-tripping to the same string) accepts. Anything else
 * — `boss`, `john..doe@corp.com`, `<a@b.co>`, `"a"@b.co` — is answered with a
 * 400, so it must never leave the browser. Non-ASCII is allowed, as Go's parser
 * allows UTF-8 atoms (RFC 6532); quoted local parts and domain literals are
 * refused, being valid but vanishingly rare for an owner lookup.
 */
const ATOM = "[\\w!#$%&'*+/=?^`{|}~\\u0080-\\uffff-]+"
const DOT_ATOM = `${ATOM}(?:\\.${ATOM})*`
const EMAIL = new RegExp(`^${DOT_ATOM}@${DOT_ATOM}$`, 'u')

/**
 * The trimmed owner email of the teams and projects lists (#1139/#1144), or
 * `undefined` when blank or not an address — a hand-edited `?owner_email=boss`
 * must not turn every reload into an error.
 */
export function ownerEmailParam(value: string | undefined): string | undefined {
  const trimmed = value?.trim() ?? ''
  return EMAIL.test(trimmed) ? trimmed : undefined
}
