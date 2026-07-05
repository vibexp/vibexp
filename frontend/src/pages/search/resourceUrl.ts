import type {
  SearchResultItem,
  SearchResultType,
} from '@/services/searchService'

export const EXCERPT_PREVIEW_LENGTH = 200

export const TYPE_LABEL: Record<SearchResultType, string> = {
  prompt: 'Prompt',
  artifact: 'Artifact',
  blueprint: 'Blueprint',
  memory: 'Memory',
}

/**
 * Human-readable title for a result. Memory results have no meaningful title,
 * so they show the type label instead.
 */
export function displayTitle(item: SearchResultItem): string {
  if (item.type === 'memory') return TYPE_LABEL.memory
  return item.title
}

/**
 * Build the in-app URL for a result, or `null` when required identifiers are
 * missing (so the caller can render a non-clickable card rather than a
 * broken link).
 *
 * Artifact and blueprint detail routes are keyed by the parent project's UUID
 * (`project_id`), not its slug — matching how the rest of the app deep-links
 * those resources (`/artifacts/:project/:slug` resolves `:project` as a UUID).
 */
export function resourceUrl(item: SearchResultItem): string | null {
  switch (item.type) {
    case 'prompt':
      return item.slug ? `/prompts/${encodeURIComponent(item.slug)}` : null
    case 'artifact':
      return item.slug && item.project_id
        ? `/artifacts/${encodeURIComponent(item.project_id)}/${encodeURIComponent(item.slug)}`
        : null
    case 'blueprint':
      return item.slug && item.project_id
        ? `/blueprints/${encodeURIComponent(item.project_id)}/${encodeURIComponent(item.slug)}`
        : null
    case 'memory':
      return `/memories/${encodeURIComponent(item.id)}`
  }
}
