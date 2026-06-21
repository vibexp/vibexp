// Platform-wide search types for the `POST /api/v1/{team_id}/search` endpoint.

/** Resource kinds that can be searched, as named in the request filter (plural). */
export type SearchFilterType =
  | 'prompts'
  | 'artifacts'
  | 'blueprints'
  | 'memories'

/** Resource kind returned for an individual result item (singular). */
export type SearchResultType = 'prompt' | 'artifact' | 'blueprint' | 'memory'

export interface SearchRequest {
  query: string
  /** Optional resource-type filter (plural names). Omit to search all types. */
  types?: SearchFilterType[]
  /** Optional project UUID filter. Omit to search across all projects. */
  project_id?: string
  page?: number
  per_page?: number
}

export interface SearchResultItem {
  type: SearchResultType
  /** Entity UUID. */
  id: string
  title: string
  /** Already truncated to <=500 chars server-side. */
  excerpt: string
  score: number
  chunk_id: string
  updated_at: string
  /** Resource slug; empty string for memory results. */
  slug: string
  /**
   * Parent project UUID. Used to build artifact and blueprint detail links
   * (those routes are keyed by project UUID, not slug). Always present.
   */
  project_id: string
  /** Human-readable parent project name, shown alongside each result. */
  project_name: string
}

export interface SearchResultsResponse {
  results: SearchResultItem[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}
