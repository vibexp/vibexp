import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the platform-wide search domain — the OpenAPI spec
// is the single source of truth; do not hand-write request/response shapes here.
export type SearchRequest = components['schemas']['SearchRequest']
export type SearchResultItem = components['schemas']['SearchResultItem']
export type SearchResultsResponse =
  components['schemas']['SearchResultsResponse']
export type SearchFilterType = NonNullable<SearchRequest['types']>[number]
export type SearchResultType = SearchResultItem['type']

/**
 * Platform-wide search service backed by `POST /api/v1/{team_id}/search`.
 *
 * Authentication is the httpOnly session cookie sent by `generatedClient`
 * (`credentials: 'include'`); no `Authorization` header is attached.
 */
class SearchService {
  async search(
    teamId: string,
    req: SearchRequest
  ): Promise<SearchResultsResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/search', {
        params: { path: { team_id: teamId } },
        body: req,
      })
    )
  }
}

export const searchService = new SearchService()
