import type { components } from '@vibexp/api-client'

type BySource = components['schemas']['ResourceCreationBySource']
type WritesBySource = components['schemas']['ResourceWritesBySource']

const NO_CREATIONS = {
  prompts: 0,
  artifacts: 0,
  blueprints: 0,
  memories: 0,
  total: 0,
}

/** A creation-metrics `by_source` with nothing created through any channel. */
export function emptyBySource(): BySource {
  return {
    web: { ...NO_CREATIONS },
    cli: { ...NO_CREATIONS },
    mcp: { ...NO_CREATIONS },
    api: { ...NO_CREATIONS },
    unknown: { ...NO_CREATIONS },
  }
}

/** A creation-metrics `writes_by_source`, zero unless a channel is given. */
export function writesBySource(
  overrides: Partial<WritesBySource> = {}
): WritesBySource {
  return { web: 0, cli: 0, mcp: 0, api: 0, unknown: 0, ...overrides }
}
