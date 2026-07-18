/** Compact absolute date for admin list/detail tables (e.g. "Jul 18, 2026"). */
export function formatAdminDate(value: string): string {
  return new Date(value).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
