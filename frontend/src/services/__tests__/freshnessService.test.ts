import { ApiError } from '../../types/errors'
import type {
  CreateFreshnessRuleRequest,
  FreshnessRule,
  TeamFreshnessSettings,
  UpdateFreshnessRuleRequest,
  UpdateTeamFreshnessSettingsRequest,
} from '../freshnessService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
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

import { freshnessService } from '../freshnessService'

const teamId = 'team-1'
const ruleId = 'rule-1'
const rulesPath = '/api/v1/{team_id}/freshness/rules'
const rulePath = '/api/v1/{team_id}/freshness/rules/{rule_id}'
const settingsPath = '/api/v1/{team_id}/settings/freshness'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContent = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response

const success = <T>(data: T, response: Response = okResponse) =>
  Promise.resolve({ data, response })

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

const rule: FreshnessRule = {
  id: ruleId,
  team_id: teamId,
  project_id: null,
  resource_types: ['artifact'],
  mediums: [],
  threshold_days: 90,
  enabled: true,
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
}

const settings: TeamFreshnessSettings = {
  source: 'instance',
  interval_seconds: 86400,
  reversibility_enabled: true,
  defaults: { interval_seconds: 86400, reversibility_enabled: true },
}

describe('freshnessService', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getRules', () => {
    it('unwraps the envelope to the bare rules array', async () => {
      mockGeneratedClient.GET.mockReturnValue(success({ rules: [rule] }))

      await expect(freshnessService.getRules(teamId)).resolves.toEqual([rule])

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(rulesPath, {
        params: { path: { team_id: teamId } },
      })
    })

    it('resolves to an empty array when the team has no rules', async () => {
      mockGeneratedClient.GET.mockReturnValue(success({ rules: [] }))

      await expect(freshnessService.getRules(teamId)).resolves.toEqual([])
    })

    it('rejects with an ApiError when the caller is not a member', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        problem(403, 'Not a member of this team', 'forbidden')
      )

      await expect(freshnessService.getRules(teamId)).rejects.toBeInstanceOf(
        ApiError
      )
    })
  })

  describe('createRule', () => {
    const request: CreateFreshnessRuleRequest = {
      project_id: null,
      resource_types: ['artifact', 'prompt'],
      mediums: ['web'],
      threshold_days: 30,
      enabled: true,
    }

    it('posts the rule to the team-scoped collection', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(rule))

      await expect(
        freshnessService.createRule(teamId, request)
      ).resolves.toEqual(rule)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(rulesPath, {
        params: { path: { team_id: teamId } },
        body: request,
      })
    })

    it('rejects with an ApiError when the server rejects the rule', async () => {
      mockGeneratedClient.POST.mockReturnValue(
        problem(400, 'threshold_days must be positive', 'validation_error')
      )

      await expect(
        freshnessService.createRule(teamId, request)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })

  describe('updateRule', () => {
    const request: UpdateFreshnessRuleRequest = {
      project_id: 'project-1',
      resource_types: ['memory'],
      mediums: [],
      threshold_days: 60,
      enabled: false,
    }

    it('puts the complete replacement at the rule path', async () => {
      mockGeneratedClient.PUT.mockReturnValue(success(rule))

      await expect(
        freshnessService.updateRule(teamId, ruleId, request)
      ).resolves.toEqual(rule)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(rulePath, {
        params: { path: { team_id: teamId, rule_id: ruleId } },
        body: request,
      })
    })

    it('rejects with an ApiError when the rule is gone', async () => {
      mockGeneratedClient.PUT.mockReturnValue(
        problem(404, 'Rule not found', 'not_found')
      )

      await expect(
        freshnessService.updateRule(teamId, ruleId, request)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })

  describe('deleteRule', () => {
    it('deletes at the rule path and resolves with nothing', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(success(undefined, noContent))

      await expect(
        freshnessService.deleteRule(teamId, ruleId)
      ).resolves.toBeUndefined()

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(rulePath, {
        params: { path: { team_id: teamId, rule_id: ruleId } },
      })
    })

    it('rejects with an ApiError when the caller may not write', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        problem(403, 'Requires team.settings.update', 'forbidden')
      )

      await expect(
        freshnessService.deleteRule(teamId, ruleId)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })

  describe('settings', () => {
    it('reads the team settings singleton', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(settings))

      await expect(freshnessService.getSettings(teamId)).resolves.toEqual(
        settings
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(settingsPath, {
        params: { path: { team_id: teamId } },
      })
    })

    it('replaces the settings in full', async () => {
      const request: UpdateTeamFreshnessSettingsRequest = {
        interval_seconds: 3600,
        reversibility_enabled: false,
      }
      mockGeneratedClient.PUT.mockReturnValue(
        success({ ...settings, source: 'team' as const, ...request })
      )

      await expect(
        freshnessService.updateSettings(teamId, request)
      ).resolves.toMatchObject({ source: 'team', interval_seconds: 3600 })

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(settingsPath, {
        params: { path: { team_id: teamId } },
        body: request,
      })
    })

    it('resets the override so the team inherits again', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(success(undefined, noContent))

      await expect(
        freshnessService.resetSettings(teamId)
      ).resolves.toBeUndefined()

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(settingsPath, {
        params: { path: { team_id: teamId } },
      })
    })

    it('rejects with an ApiError when the interval is below the floor', async () => {
      mockGeneratedClient.PUT.mockReturnValue(
        problem(
          400,
          'interval_seconds must be at least 3600',
          'validation_error'
        )
      )

      await expect(
        freshnessService.updateSettings(teamId, {
          interval_seconds: 60,
          reversibility_enabled: true,
        })
      ).rejects.toBeInstanceOf(ApiError)
    })
  })
})
