import type { components } from '@vibexp/api-client'

import {
  generatedClient,
  longRunningClient,
  unwrap,
} from '../lib/apiClientGenerated'

// Generated wire types for the platform-wide search domain — the OpenAPI spec
// is the single source of truth; do not hand-write request/response shapes here.
export type SearchRequest = components['schemas']['SearchRequest']
export type SearchResultItem = components['schemas']['SearchResultItem']
export type SearchResultsResponse =
  components['schemas']['SearchResultsResponse']
export type SearchFilterType = NonNullable<SearchRequest['types']>[number]
export type SearchResultType = SearchResultItem['type']
export type SearchAISummaryAvailability =
  components['schemas']['SearchAISummaryAvailability']
export type SearchSummaryRequest = components['schemas']['SearchSummaryRequest']
export type SearchSummaryResponse =
  components['schemas']['SearchSummaryResponse']
export type SearchSummarySource = components['schemas']['SearchSummarySource']

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

  /**
   * Generate an AI Summary of the team's top search results for a query
   * (`POST /api/v1/{team_id}/search/summary`, operation
   * `summarizeSearchResults`). The backend waits on a model provider for up to
   * its own request timeout, so this call uses the long-running client rather
   * than the 30s default.
   */
  async summarize(
    teamId: string,
    req: SearchSummaryRequest
  ): Promise<SearchSummaryResponse> {
    return unwrap(
      longRunningClient.POST('/api/v1/{team_id}/search/summary', {
        params: { path: { team_id: teamId } },
        body: req,
      })
    )
  }
}

export const searchService = new SearchService()
