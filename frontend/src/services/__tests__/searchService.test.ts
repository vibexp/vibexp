import type { SearchRequest, SearchResultsResponse } from '../searchService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  POST: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { searchService } from '../searchService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

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
    mockGeneratedClient.POST.mockReturnValue(success(response))
    const req: SearchRequest = { query: 'hello', page: 1, per_page: 20 }

    const result = await searchService.search(teamId, req)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/search',
      { params: { path: { team_id: teamId } }, body: req }
    )
    expect(result).toEqual(response)
  })

  it('forwards an optional types filter unchanged', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(response))
    const req: SearchRequest = {
      query: 'hello',
      types: ['prompts', 'memories'],
      page: 1,
      per_page: 20,
    }

    await searchService.search(teamId, req)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/search',
      { params: { path: { team_id: teamId } }, body: req }
    )
  })

  it('forwards an optional project_id filter unchanged', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(response))
    const req: SearchRequest = {
      query: 'hello',
      project_id: '7c9e6679-7425-40de-944b-e07fc1f90ae7',
      page: 1,
      per_page: 20,
    }

    await searchService.search(teamId, req)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/search',
      { params: { path: { team_id: teamId } }, body: req }
    )
  })

  it('throws ApiError with backend detail on RFC 9457 error', async () => {
    mockGeneratedClient.POST.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'https://api.vibexp.io/errors/BAD_REQUEST',
          title: 'Bad Request',
          status: 400,
          detail: 'query is required',
          code: 'BAD_REQUEST',
          request_id: 'req-1',
          timestamp: '2024-01-01T10:00:00Z',
        },
        response: { ok: false, status: 400, statusText: 'Bad Request' },
      })
    )

    await expect(
      searchService.search(teamId, { query: '', page: 1, per_page: 20 })
    ).rejects.toThrow('query is required')
  })
})
