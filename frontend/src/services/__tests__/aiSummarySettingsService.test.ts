import { ApiError } from '../../types/errors'
import type {
  TeamAISummarySettings,
  UpdateTeamAISummarySettingsRequest,
} from '../aiSummarySettingsService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = vi.hoisted(() => ({
  GET: vi.fn(),
  PUT: vi.fn(),
  DELETE: vi.fn(),
}))

vi.mock('../../lib/apiClientGenerated', async () => {
  const actual = await vi.importActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { aiSummarySettingsService } from '../aiSummarySettingsService'

const teamId = 'team-1'
const path = '/api/v1/{team_id}/settings/ai-summary'

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
  enabled: false,
  model_provider_id: null,
  top_n: 5,
  style: 'balanced' as const,
  max_output_tokens: 800,
}

const settings: TeamAISummarySettings = {
  source: 'instance',
  values: instanceDefaults,
  instance_defaults: instanceDefaults,
  max_top_n: 10,
  available: true,
}

describe('aiSummarySettingsService', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('GETs the team-scoped settings and resolves the payload', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(settings))

    await expect(
      aiSummarySettingsService.getAISummarySettings(teamId)
    ).resolves.toEqual(settings)
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(path, {
      params: { path: { team_id: teamId } },
    })
  })

  it('PUTs the complete profile and resolves the stored settings', async () => {
    const request: UpdateTeamAISummarySettingsRequest = {
      enabled: true,
      model_provider_id: 'provider-1',
      top_n: 3,
      style: 'concise',
      max_output_tokens: 400,
    }
    const stored: TeamAISummarySettings = {
      ...settings,
      source: 'team',
      values: request,
    }
    mockGeneratedClient.PUT.mockReturnValue(success(stored))

    await expect(
      aiSummarySettingsService.updateAISummarySettings(teamId, request)
    ).resolves.toEqual(stored)
    expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(path, {
      params: { path: { team_id: teamId } },
      body: request,
    })
  })

  it('rejects a PUT the server refuses with an ApiError carrying its message', async () => {
    mockGeneratedClient.PUT.mockReturnValue(
      problem(400, 'top_n exceeds max_top_n', 'invalid_request')
    )

    const result = aiSummarySettingsService.updateAISummarySettings(
      teamId,
      instanceDefaults
    )
    await expect(result).rejects.toBeInstanceOf(ApiError)
    await expect(result).rejects.toThrow('top_n exceeds max_top_n')
  })

  it('DELETEs the override and resolves on 204', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(success(undefined, noContent))

    await expect(
      aiSummarySettingsService.resetAISummarySettings(teamId)
    ).resolves.toBeUndefined()
    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(path, {
      params: { path: { team_id: teamId } },
    })
  })
})
