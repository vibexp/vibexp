/**
 * Reads a field's value off an API payload, following a dotted key one level
 * down (`source.commit_sha`) so a nested provenance object can be described as
 * ordinary fields instead of needing a bespoke renderer per page.
 *
 * Shared by the descriptor-driven sections; not part of the package's public
 * surface, which is descriptors and components rather than payload plumbing.
 */
export function valueOf(
  resource: Record<string, unknown>,
  key: string
): unknown {
  const path = key.split('.')
  let current: unknown = resource
  for (const segment of path) {
    if (current === null || typeof current !== 'object') return undefined
    current = new Map(Object.entries(current)).get(segment)
  }
  return current
}
