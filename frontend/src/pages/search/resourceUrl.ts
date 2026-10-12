import { buildResourceUrl } from '@/lib/resourceUrl'
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
 * Human-readable title for a result: the name the server returns — for a
 * memory, its title or a body-derived stand-in. The type label is only the
 * last resort, for a result that comes back with no name at all.
 */
export function displayTitle(item: SearchResultItem): string {
  return item.title.trim() || TYPE_LABEL[item.type] || item.type
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
  return buildResourceUrl({
    type: item.type,
    id: item.id,
    slug: item.slug,
    projectId: item.project_id,
  })
}
