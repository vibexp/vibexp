import { apiClient } from '../lib/apiClient'
import type { SearchRequest, SearchResultsResponse } from '../types/search'

/**
 * Platform-wide search service backed by `POST /api/v1/{team_id}/search`.
 *
 * Authentication is the httpOnly session cookie sent by `apiClient`
 * (`credentials: 'include'`); no `Authorization` header is attached.
 */
class SearchService {
  async search(
    teamId: string,
    req: SearchRequest
  ): Promise<SearchResultsResponse> {
    return apiClient.post<SearchResultsResponse>(
      `/${encodeURIComponent(teamId)}/search`,
      req
    )
  }
}

export const searchService = new SearchService()
