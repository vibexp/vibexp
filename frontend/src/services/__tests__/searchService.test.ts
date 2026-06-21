import type { SearchRequest, SearchResultsResponse } from '../../types/search'

const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
  put: jest.fn(),
  delete: jest.fn(),
}

jest.mock('../../lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

import { searchService } from '../searchService'

describe('SearchService', () => {
  const teamId = 'team-123'

  const response: SearchResultsResponse = {
    results: [
      {
        type: 'prompt',
        id: 'prompt-1',
        title: 'My Prompt',
        excerpt: 'an excerpt',
        score: 0.9,
        chunk_id: 'chunk-1',
        updated_at: '2024-01-01T00:00:00Z',
        slug: 'my-prompt',
        project_id: '7c9e6679-7425-40de-944b-e07fc1f90ae7',
        project_name: 'My Project',
      },
    ],
    total_count: 1,
    page: 1,
    per_page: 20,
    total_pages: 1,
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('posts to the team-scoped search endpoint with the request payload', async () => {
    mockApiClient.post.mockResolvedValue(response)
    const req: SearchRequest = { query: 'hello', page: 1, per_page: 20 }

    const result = await searchService.search(teamId, req)

    expect(mockApiClient.post).toHaveBeenCalledWith(`/${teamId}/search`, req)
    expect(result).toEqual(response)
  })

  it('url-encodes the team id in the endpoint path', async () => {
    mockApiClient.post.mockResolvedValue(response)

    await searchService.search('team/with space', { query: 'x' })

    expect(mockApiClient.post).toHaveBeenCalledWith(
      '/team%2Fwith%20space/search',
      { query: 'x' }
    )
  })

  it('forwards an optional types filter unchanged', async () => {
    mockApiClient.post.mockResolvedValue(response)
    const req: SearchRequest = {
      query: 'hello',
      types: ['prompts', 'memories'],
    }

    await searchService.search(teamId, req)

    expect(mockApiClient.post).toHaveBeenCalledWith(`/${teamId}/search`, req)
  })

  it('forwards an optional project_id filter unchanged', async () => {
    mockApiClient.post.mockResolvedValue(response)
    const req: SearchRequest = {
      query: 'hello',
      project_id: '7c9e6679-7425-40de-944b-e07fc1f90ae7',
    }

    await searchService.search(teamId, req)

    expect(mockApiClient.post).toHaveBeenCalledWith(`/${teamId}/search`, req)
  })

  it('propagates errors thrown by the api client', async () => {
    mockApiClient.post.mockRejectedValue(new Error('boom'))

    await expect(
      searchService.search(teamId, { query: 'hello' })
    ).rejects.toThrow('boom')
  })
})
