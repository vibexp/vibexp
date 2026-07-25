/**
 * Bytes as a short human-readable size, 1024-based to match how Postgres reports
 * `pg_database_size`.
 *
 * Lives outside the panel component so it is directly testable and so the panel
 * file stays components-only for Fast Refresh.
 */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${String(bytes)} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  // One decimal below 10 so "1.5 MiB" is not rounded to "2 MiB"; none above,
  // where the extra digit is noise.
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[unit]}`
}
