/**
 * The four resource kinds the SPA deep-links to. A bare `string` is accepted so
 * callers can pass a wire field directly; an unrecognized type yields `null`.
 */
export type ResourceUrlType = 'prompt' | 'artifact' | 'blueprint' | 'memory'

/**
 * The minimal server-resolved fields needed to build a resource's detail URL.
 * Artifact/blueprint links are keyed by the parent project's UUID (`projectId`),
 * matching how the rest of the app deep-links those resources; prompts are keyed
 * by slug alone; memories by their own id.
 */
export interface ResourceUrlFields {
  /** A resource kind; accepts a raw wire string, `null` for anything unknown. */
  type: string
  id: string
  slug?: string | null
  projectId?: string | null
}

/**
 * Build the in-app URL for a resource, or `null` when required identifiers are
 * missing (so the caller can render a non-clickable element rather than a broken
 * link). Single source of truth for resource deep-links — both the search page
 * and the homepage recent-comments card resolve through here.
 */
export function buildResourceUrl(fields: ResourceUrlFields): string | null {
  const { type, id, slug, projectId } = fields
  switch (type) {
    case 'prompt':
      return slug ? `/prompts/${encodeURIComponent(slug)}` : null
    case 'artifact':
      return slug && projectId
        ? `/artifacts/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}`
        : null
    case 'blueprint':
      return slug && projectId
        ? `/blueprints/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}`
        : null
    case 'memory':
      return `/memories/${encodeURIComponent(id)}`
    default:
      return null
  }
}
