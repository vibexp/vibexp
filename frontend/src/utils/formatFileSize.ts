/**
 * Formats a byte count as a human-readable size (e.g. 1536 -> "1.5 KB").
 * Uses binary (1024) units to match the backend's MiB-based limits.
 */
export function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const k = 1024
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(k)),
    units.length - 1
  )
  const value = bytes / Math.pow(k, i)
  // Whole bytes show no decimal; larger units show up to one decimal place.
  const formatted = i === 0 ? String(value) : value.toFixed(1)
  return `${formatted} ${units.at(i) ?? 'B'}`
}
