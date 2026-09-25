/**
 * Client-side mirror of the server's saved-filter preset rules (#1147), so a
 * bad name is caught inline before any request. The server still validates
 * everything and its 400 `detail` is shown when it disagrees.
 */

/** Presets one admin can keep per list. */
export const MAX_PRESETS = 20

/** Longest preset name, in characters after trimming. */
export const MAX_NAME = 80

/**
 * Why `name` cannot be used, or `null` when it can.
 *
 * Uniqueness is case-insensitive, like the server's. `exceptId` excludes the
 * preset being renamed, so keeping (or re-casing) its own name is allowed.
 */
export function validatePresetName(
  name: string,
  existing: readonly { id: string; name: string }[],
  exceptId?: string
): string | null {
  const trimmed = name.trim()
  if (trimmed === '') return 'Name is required'
  // Count code points, not UTF-16 units: the server counts runes.
  if (Array.from(trimmed).length > MAX_NAME) {
    return `Name must be ${String(MAX_NAME)} characters or fewer`
  }
  const lower = trimmed.toLowerCase()
  const clash = existing.some(
    preset =>
      preset.id !== exceptId && preset.name.trim().toLowerCase() === lower
  )
  return clash ? 'A preset with this name already exists' : null
}

/** Whether two preset queries select the same filters, ignoring key order. */
export function sameQuery(
  a: Readonly<Record<string, string>>,
  b: Readonly<Record<string, string>>
): boolean {
  const aKeys = Object.keys(a)
  if (aKeys.length !== Object.keys(b).length) return false
  return aKeys.every(key => Object.hasOwn(b, key) && a[key] === b[key])
}
