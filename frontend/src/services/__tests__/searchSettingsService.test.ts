import { ApiError } from '../../types/errors'
import type {
  TeamSearchSettings,
  UpdateTeamSearchSettingsRequest,
} from '../searchSettingsService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  PUT: jest.fn(),
  DELETE: jest.fn(),
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

import { searchSettingsService } from '../searchSettingsService'

const teamId = 'team-1'
const path = '/api/v1/{team_id}/settings/search'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContent = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response

const success = <T>(data: T, response: Response = okResponse) =>
  Promise.resolve({ data, response })

// An RFC 9457 problem-details error body as openapi-fetch surfaces it.
const problem = (status: number, detail: string, code: string) =>
  Promise.resolve({
    error: {
      type: `https://api.vibexp.io/errors/${code}`,
      title: code,
      status,
      detail,
      code,
      request_id: 'req-1',
      timestamp: '2024-01-01T00:00:00Z',
    },
    response: { ok: false, status, statusText: code } as Response,
  })

const instanceDefaults = {
  recency_ranking_enabled: true,
  rank_weight_relevance: 0.5,
  rank_weight_created: 0.3,
  rank_weight_updated: 0.2,
  rank_half_life_days: 90,
}

const settings: TeamSearchSettings = {
  source: 'instance',
  ...instanceDefaults,
  instance_defaults: instanceDefaults,
  rank_candidate_cap: 200,
}

describe('searchSettingsService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getSearchSettings', () => {
    it('requests the team-scoped settings and resolves the payload', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(settings))

      await expect(
        searchSettingsService.getSearchSettings(teamId)
      ).resolves.toEqual(settings)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(path, {
        params: { path: { team_id: teamId } },
      })
    })

    it('rejects with an ApiError when the caller is not a member', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        problem(403, 'Not a member of this team', 'forbidden')
      )

      await expect(
        searchSettingsService.getSearchSettings(teamId)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })

  describe('updateSearchSettings', () => {
    const request: UpdateTeamSearchSettingsRequest = {
      recency_ranking_enabled: true,
      rank_weight_relevance: 0.3,
      rank_weight_created: 0.4,
      rank_weight_updated: 0.3,
      rank_half_life_days: 30,
    }

    it('PUTs the complete profile and resolves the stored settings', async () => {
      const stored: TeamSearchSettings = {
        source: 'team',
        ...request,
        instance_defaults: instanceDefaults,
        rank_candidate_cap: 200,
      }
      mockGeneratedClient.PUT.mockReturnValue(success(stored))

      await expect(
        searchSettingsService.updateSearchSettings(teamId, request)
      ).resolves.toEqual(stored)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(path, {
        params: { path: { team_id: teamId } },
        body: request,
      })
    })

    it('rejects with an ApiError when the profile is invalid', async () => {
      mockGeneratedClient.PUT.mockReturnValue(
        problem(400, 'weights must not be negative', 'invalid_request')
      )

      await expect(
        searchSettingsService.updateSearchSettings(teamId, request)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })

  describe('resetSearchSettings', () => {
    it('DELETEs the override and resolves on 204', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(success(undefined, noContent))

      await expect(
        searchSettingsService.resetSearchSettings(teamId)
      ).resolves.toBeUndefined()

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(path, {
        params: { path: { team_id: teamId } },
      })
    })

    it('rejects with an ApiError when the caller lacks the permission', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        problem(403, 'team.settings.update required', 'forbidden')
      )

      await expect(
        searchSettingsService.resetSearchSettings(teamId)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })
})
