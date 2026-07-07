import type {
  Agent,
  AgentExecution,
  AgentListResponse,
  AgentStatsResponse,
  CreateAgentRequest,
} from '../agentService'

const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  PUT: jest.fn(),
  DELETE: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return { ...actual, generatedClient: mockGeneratedClient }
})

import { agentService } from '../agentService'

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
      timestamp: '2026-01-01T00:00:00Z',
    },
    response: { ok: false, status, statusText: code } as Response,
  })

const teamId = 'team-1'
const agentId = 'agent-1'

const agent: Agent = {
  id: agentId,
  user_id: 'user-1',
  team_id: teamId,
  name: 'My Agent',
  description: 'desc',
  status: 'active',
  config: {},
  total_runs: 0,
  success_rate: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  version: 1,
}

const execution: AgentExecution = {
  id: 'exec-1',
  agent_id: agentId,
  user_id: 'user-1',
  status: 'running',
  started_at: '2026-01-01T00:00:00Z',
  version: 1,
}

describe('AgentService (generatedClient)', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('getAgents GETs the team-scoped list with filters as query params (bare list response)', async () => {
    const list: AgentListResponse = {
      agents: [agent],
      page: 1,
      per_page: 20,
      total_count: 1,
      total_pages: 1,
    }
    mockGeneratedClient.GET.mockReturnValue(success(list))

    const result = await agentService.getAgents(teamId, {
      status: 'active',
      page: 1,
      limit: 20,
    })

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/agents',
      {
        params: {
          path: { team_id: teamId },
          query: {
            status: 'active',
            search: undefined,
            page: 1,
            limit: 20,
          },
        },
      }
    )
    expect(result).toEqual(list)
  })

  it('getAgent GETs by id and returns the bare agent', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(agent))

    const result = await agentService.getAgent(teamId, agentId)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/agents/{id}',
      { params: { path: { team_id: teamId, id: agentId } } }
    )
    expect(result).toEqual(agent)
  })

  it('createAgent POSTs the request body', async () => {
    const req: CreateAgentRequest = { card_url: 'https://agent.example/card' }
    mockGeneratedClient.POST.mockReturnValue(success(agent))

    await agentService.createAgent(teamId, req)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/agents',
      { params: { path: { team_id: teamId } }, body: req }
    )
  })

  it('getAgentStats returns the bare stats (no envelope)', async () => {
    const stats: AgentStatsResponse = {
      total_agents: 3,
      active_agents: 2,
      paused_agents: 1,
      error_agents: 0,
      total_runs: 10,
      avg_success_rate: 0.9,
      runs_today: 1,
      runs_this_week: 5,
      recent_activities: [],
    }
    mockGeneratedClient.GET.mockReturnValue(success(stats))

    const result = await agentService.getAgentStats(teamId)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/agents/stats',
      { params: { path: { team_id: teamId } } }
    )
    expect(result).toEqual(stats)
  })

  it('executeAgent POSTs input + conversation_id and returns the bare execution', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(execution))

    const result = await agentService.executeAgent(
      teamId,
      agentId,
      { text: 'hi' },
      'conv-1'
    )

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/agents/{id}/execute',
      {
        params: { path: { team_id: teamId, id: agentId } },
        body: { input: { text: 'hi' }, conversation_id: 'conv-1' },
      }
    )
    expect(result).toEqual(execution)
  })

  it('completeAgentExecution PUTs to the execution path with the narrowed status body', async () => {
    mockGeneratedClient.PUT.mockReturnValue(success(execution))

    await agentService.completeAgentExecution(teamId, 'exec-1', {
      status: 'success',
    })

    expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
      '/api/v1/{team_id}/agents/executions/{execution_id}',
      {
        params: { path: { team_id: teamId, execution_id: 'exec-1' } },
        body: { status: 'success' },
      }
    )
  })

  it('deleteAgent DELETEs by id and resolves void', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(success(undefined, noContent))

    await expect(
      agentService.deleteAgent(teamId, agentId)
    ).resolves.toBeUndefined()
    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      '/api/v1/{team_id}/agents/{id}',
      { params: { path: { team_id: teamId, id: agentId } } }
    )
  })

  it('throws ApiError with the backend detail on failure', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      problem(404, 'Agent not found', 'AGENT_NOT_FOUND')
    )

    await expect(agentService.getAgent(teamId, 'missing')).rejects.toThrow(
      'Agent not found'
    )
  })
})
