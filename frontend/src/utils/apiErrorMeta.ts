import { ApiError } from '@/types/errors'

/**
 * Read the first non-empty string value among the given RFC 9457 metadata
 * keys of an ApiError. Returns undefined when the metadata is absent, the
 * key is missing, or the value is not a non-empty string.
 */
export function readStringMeta(
  error: ApiError,
  ...keys: string[]
): string | undefined {
  const meta = error.metadata
  if (!meta) return undefined
  for (const key of keys) {
    const value = Object.prototype.hasOwnProperty.call(meta, key)
      ? meta[key]
      : undefined
    if (typeof value === 'string' && value.length > 0) {
      return value
    }
  }
  return undefined
}
