/**
 * The CSV export query for a list query: the same filters and sort, without
 * pagination (#1150). Each page derives it from the one object it sends to the
 * list call, so the export and the table on screen cannot disagree.
 */
export function withoutPaging<T extends { page?: number; limit?: number }>(
  params: T
): Omit<T, 'page' | 'limit'> {
  const { page: _page, limit: _limit, ...rest } = params
  return rest
}
