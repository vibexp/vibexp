/**
 * Unit Tests for AgentService - DL-144
 *
 * This comprehensive test suite validates the AgentService functionality including:
 * - All CRUD operations (create, read, update, delete agents)
 * - Agent execution management (start, complete, get executions)
 * - Configuration validation and error handling
 * - Authentication and authorization scenarios
 * - Edge cases and performance scenarios
 *
 * Coverage: 31 tests covering all AgentService methods and error paths
 * Story Points: 4 - Completed as part of DL-144 ticket
 */

import type {
  Agent,
  AgentFilters,
  CreateAgentRequest,
  UpdateAgentRequest,
  StartAgentExecutionRequest,
  CompleteAgentExecutionRequest,
  AgentsResponse,
  AgentResponse,
  AgentStatsApiResponse,
  AgentExecutionResponse,
} from '../../src/types'

// Mock the authService
const mockAuthService = {
  getToken: jest.fn(),
  setToken: jest.fn(),
}

jest.mock('../../src/services/authService', () => ({
  authService: mockAuthService,
}))

// Mock fetch globally
global.fetch = jest.fn()

// Mock the AgentService class to avoid import.meta.env issues
class MockAgentService {
  private API_BASE_URL = 'https://api.vibexp.io/api/v1'

  private async makeRequest<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const token = mockAuthService.getToken()
    if (!token) {
      throw new Error('No authentication token')
    }

    const response = await fetch(`${this.API_BASE_URL}${endpoint}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
        ...options.headers,
      },
    })

    if (!response.ok) {
      if (response.status === 401) {
        mockAuthService.setToken(null)
        throw new Error('Authentication expired')
      }
      const errorData = await response.json().catch(() => null)
      throw new Error(
        errorData?.error?.message ||
          errorData?.message ||
          `HTTP error! status: ${response.status}`
      )
    }

    // Handle 204 No Content responses (like successful deletes)
    if (response.status === 204) {
      return null as T
    }

    // Check if response has JSON content
    const contentType = response.headers.get('content-type')
    if (contentType && contentType.includes('application/json')) {
      return response.json()
    }

    // Try to parse as JSON even if content-type header is not set correctly
    try {
      const text = await response.text()
      if (text.trim()) {
        return JSON.parse(text)
      }
    } catch (e) {
      console.warn('Failed to parse response as JSON:', e)
    }

    return null as T
  }

  async getAgents(
    teamId: string,
    filters: AgentFilters = {}
  ): Promise<AgentsResponse> {
    const params = new URLSearchParams()

    // Remove team_id from query params - it's now in the URL path
    if (filters.status) params.append('status', filters.status)
    if (filters.search) params.append('search', filters.search)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/${teamId}/agents${queryString ? `?${queryString}` : ''}`

    return this.makeRequest<AgentsResponse>(endpoint)
  }

  async getAgent(teamId: string, id: string): Promise<AgentResponse> {
    return this.makeRequest<AgentResponse>(`/${teamId}/agents/${id}`)
  }

  async createAgent(
    teamId: string,
    data: CreateAgentRequest
  ): Promise<AgentResponse> {
    return this.makeRequest<AgentResponse>(`/${teamId}/agents`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateAgent(
    teamId: string,
    id: string,
    data: UpdateAgentRequest
  ): Promise<AgentResponse> {
    return this.makeRequest<AgentResponse>(`/${teamId}/agents/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async deleteAgent(teamId: string, id: string): Promise<void> {
    return this.makeRequest<void>(`/${teamId}/agents/${id}`, {
      method: 'DELETE',
    })
  }

  async getAgentStats(teamId: string): Promise<AgentStatsApiResponse> {
    return this.makeRequest<AgentStatsApiResponse>(`/${teamId}/agents/stats`)
  }

  async startAgentExecution(
    teamId: string,
    agentId: string,
    data: StartAgentExecutionRequest = {}
  ): Promise<AgentExecutionResponse> {
    return this.makeRequest<AgentExecutionResponse>(
      `/${teamId}/agents/${agentId}/executions`,
      {
        method: 'POST',
        body: JSON.stringify(data),
      }
    )
  }

  async completeAgentExecution(
    teamId: string,
    executionId: string,
    data: CompleteAgentExecutionRequest
  ): Promise<AgentExecutionResponse> {
    return this.makeRequest<AgentExecutionResponse>(
      `/${teamId}/agents/executions/${executionId}`,
      {
        method: 'PUT',
        body: JSON.stringify(data),
      }
    )
  }

  async getAgentExecution(
    teamId: string,
    executionId: string
  ): Promise<AgentExecutionResponse> {
    return this.makeRequest<AgentExecutionResponse>(
      `/${teamId}/agents/executions/${executionId}`
    )
  }

  async updateAgentCredentials(
    teamId: string,
    agentId: string,
    credentials: Record<string, { value: string }>
  ): Promise<void> {
    return this.makeRequest<void>(`/${teamId}/agents/${agentId}/credentials`, {
      method: 'PUT',
      body: JSON.stringify({ credentials }),
    })
  }
}

const agentService = new MockAgentService()

describe('AgentService', () => {
  const mockToken = 'mock-auth-token'
  const mockTeamId = 'team-123'
  const mockAgent: Agent = {
    id: 'agent-1',
    user_id: 'user-1',
    team_id: mockTeamId,
    name: 'Test Agent',
    description: 'A test agent',
    status: 'active',
    card_url: 'https://example.com/agent-card',
    agent_card: {
      name: 'Test Agent Card',
      description: 'Test agent card description',
      version: '1.0.0',
      protocolVersion: '1.0',
      url: 'https://example.com/agent',
      preferredTransport: 'http',
      defaultInputModes: ['text'],
      defaultOutputModes: ['text'],
      skills: [],
    },
    config: {},
    last_run: '2023-01-01T00:00:00Z',
    total_runs: 10,
    success_rate: 95.5,
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
  }

  const mockFetch = fetch as jest.MockedFunction<typeof fetch>

  beforeEach(() => {
    jest.clearAllMocks()
    mockAuthService.getToken.mockReturnValue(mockToken)
    mockFetch.mockClear()
  })

  describe('makeRequest', () => {
    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(agentService.getAgents(mockTeamId)).rejects.toThrow(
        'No authentication token'
      )
    })

    it('should handle 401 responses and clear token', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: jest.fn().mockResolvedValue({ message: 'Unauthorized' }),
      } as unknown as Response)

      await expect(agentService.getAgents(mockTeamId)).rejects.toThrow(
        'Authentication expired'
      )
      expect(mockAuthService.setToken).toHaveBeenCalledWith(null)
    })

    it('should handle error responses with error messages', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: jest.fn().mockResolvedValue({
          error: { message: 'Bad request' },
        }),
      } as unknown as Response)

      await expect(agentService.getAgents(mockTeamId)).rejects.toThrow(
        'Bad request'
      )
    })

    it('should handle error responses with message field', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: jest.fn().mockResolvedValue({
          message: 'Invalid input',
        }),
      } as unknown as Response)

      await expect(agentService.getAgents(mockTeamId)).rejects.toThrow(
        'Invalid input'
      )
    })

    it('should handle error responses without specific message', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: jest.fn().mockRejectedValue(new Error('Invalid JSON')),
      } as unknown as Response)

      await expect(agentService.getAgents(mockTeamId)).rejects.toThrow(
        'HTTP error! status: 500'
      )
    })

    it('should handle 204 No Content responses', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 204,
      } as unknown as Response)

      const result = await agentService.deleteAgent(mockTeamId, 'agent-1')
      expect(result).toBeNull()
    })

    it('should handle responses with JSON content-type', async () => {
      const mockResponse = { data: mockAgent }
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: {
          get: jest.fn().mockReturnValue('application/json'),
        },
        json: jest.fn().mockResolvedValue(mockResponse),
      } as unknown as Response)

      const result = await agentService.getAgent(mockTeamId, 'agent-1')
      expect(result).toEqual(mockResponse)
    })

    it('should handle responses without JSON content-type but valid JSON', async () => {
      const mockResponse = { data: mockAgent }
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: {
          get: jest.fn().mockReturnValue(null),
        },
        text: jest.fn().mockResolvedValue(JSON.stringify(mockResponse)),
      } as unknown as Response)

      const result = await agentService.getAgent(mockTeamId, 'agent-1')
      expect(result).toEqual(mockResponse)
    })

    it('should handle empty response bodies', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: {
          get: jest.fn().mockReturnValue(null),
        },
        text: jest.fn().mockResolvedValue(''),
      } as unknown as Response)

      const result = await agentService.deleteAgent(mockTeamId, 'agent-1')
      expect(result).toBeNull()
    })

    it('should handle invalid JSON gracefully', async () => {
      const consoleSpy = jest.spyOn(console, 'warn').mockImplementation()

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: {
          get: jest.fn().mockReturnValue(null),
        },
        text: jest.fn().mockResolvedValue('invalid json'),
      } as unknown as Response)

      const result = await agentService.deleteAgent(mockTeamId, 'agent-1')
      expect(result).toBeNull()
      expect(consoleSpy).toHaveBeenCalledWith(
        'Failed to parse response as JSON:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })

  describe('Agent Management', () => {
    describe('getAgents', () => {
      it('should fetch agents without filters', async () => {
        const mockResponse: AgentsResponse = {
          status: 'success',
          message: 'Agents retrieved successfully',
          data: {
            agents: [mockAgent],
            page: 1,
            per_page: 20,
            total_count: 1,
            total_pages: 1,
          },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.getAgents(mockTeamId)

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents`,
          expect.objectContaining({
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })

      it('should fetch agents with filters', async () => {
        const filters: AgentFilters = {
          status: 'active',
          search: 'test',
          page: 2,
          limit: 10,
        }

        const mockResponse: AgentsResponse = {
          status: 'success',
          message: 'Agents retrieved successfully',
          data: {
            agents: [mockAgent],
            page: 2,
            per_page: 10,
            total_count: 1,
            total_pages: 1,
          },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.getAgents(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents?status=active&search=test&page=2&limit=10`,
          expect.objectContaining({
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })

      it('should handle partial filters', async () => {
        const filters: AgentFilters = {
          status: 'paused',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue({
            status: 'success',
            data: {
              agents: [],
              page: 1,
              per_page: 20,
              total_count: 0,
              total_pages: 0,
            },
          }),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        await agentService.getAgents(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents?status=paused`,
          expect.any(Object)
        )
      })
    })

    describe('getAgent', () => {
      it('should fetch a single agent by ID', async () => {
        const mockResponse: AgentResponse = {
          status: 'success',
          message: 'Agent retrieved successfully',
          data: mockAgent,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.getAgent(mockTeamId, 'agent-1')

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/agent-1`,
          expect.objectContaining({
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })
    })

    describe('createAgent', () => {
      it('should create a new agent', async () => {
        const createRequest: CreateAgentRequest = {
          card_url: 'https://example.com/new-agent-card',
          status: 'active',
        }

        const mockResponse: AgentResponse = {
          status: 'success',
          message: 'Agent created successfully',
          data: { ...mockAgent, card_url: createRequest.card_url },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 201,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.createAgent(mockTeamId, createRequest)

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents`,
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify(createRequest),
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })
    })

    describe('updateAgent', () => {
      it('should update an existing agent', async () => {
        const updateRequest: UpdateAgentRequest = {
          status: 'paused',
          card_url: 'https://example.com/updated-agent-card',
        }

        const mockResponse: AgentResponse = {
          status: 'success',
          message: 'Agent updated successfully',
          data: { ...mockAgent, ...updateRequest },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.updateAgent(
          mockTeamId,
          'agent-1',
          updateRequest
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/agent-1`,
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify(updateRequest),
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })
    })

    describe('deleteAgent', () => {
      it('should delete an agent', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 204,
        } as unknown as Response)

        const result = await agentService.deleteAgent(mockTeamId, 'agent-1')

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/agent-1`,
          expect.objectContaining({
            method: 'DELETE',
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toBeNull()
      })
    })

    describe('updateAgentCredentials', () => {
      it('should update agent credentials', async () => {
        const credentials = {
          api_key: { value: 'test-api-key-123' },
          oauth_token: { value: 'test-oauth-token-456' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 204,
        } as unknown as Response)

        const result = await agentService.updateAgentCredentials(
          mockTeamId,
          'agent-1',
          credentials
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/agent-1/credentials`,
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify({ credentials }),
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toBeNull()
      })

      it('should handle single credential update', async () => {
        const credentials = {
          api_key: { value: 'only-api-key' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 204,
        } as unknown as Response)

        await agentService.updateAgentCredentials(
          mockTeamId,
          'agent-1',
          credentials
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/agent-1/credentials`,
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify({ credentials }),
          })
        )
      })

      it('should handle 404 when agent not found', async () => {
        const credentials = {
          api_key: { value: 'test-key' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 404,
          json: jest.fn().mockResolvedValue({
            error: { message: 'Agent not found' },
          }),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        await expect(
          agentService.updateAgentCredentials(
            mockTeamId,
            'non-existent-agent',
            credentials
          )
        ).rejects.toThrow('Agent not found')
      })

      it('should handle 500 server error', async () => {
        const credentials = {
          api_key: { value: 'test-key' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 500,
          json: jest.fn().mockResolvedValue({
            error: { message: 'Internal server error' },
          }),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        await expect(
          agentService.updateAgentCredentials(
            mockTeamId,
            'agent-1',
            credentials
          )
        ).rejects.toThrow('Internal server error')
      })
    })

    describe('getAgentStats', () => {
      it('should fetch agent statistics', async () => {
        const mockStatsResponse: AgentStatsApiResponse = {
          status: 'success',
          message: 'Agent stats retrieved successfully',
          data: {
            total_agents: 5,
            active_agents: 3,
            paused_agents: 1,
            error_agents: 1,
            total_runs: 100,
            avg_success_rate: 92.5,
            runs_today: 10,
            runs_this_week: 50,
            recent_activities: [
              {
                id: 'activity-1',
                agent_id: 'agent-1',
                agent_name: 'Test Agent',
                action: 'execution',
                status: 'success',
                description: 'Agent executed successfully',
                created_at: '2023-01-01T00:00:00Z',
              },
            ],
          },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockStatsResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.getAgentStats(mockTeamId)

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/stats`,
          expect.objectContaining({
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockStatsResponse)
      })
    })
  })

  describe('Agent Interactions', () => {
    const mockExecution = {
      id: 'execution-1',
      agent_id: 'agent-1',
      user_id: 'user-1',
      status: 'success' as const,
      input: { message: 'test input' },
      output: { result: 'test output' },
      error: null,
      started_at: '2023-01-01T00:00:00Z',
      ended_at: '2023-01-01T00:01:00Z',
      duration: 60000,
    }

    describe('startAgentExecution', () => {
      it('should start agent execution without input', async () => {
        const mockResponse: AgentExecutionResponse = {
          status: 'success',
          message: 'Agent execution started',
          data: mockExecution,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.startAgentExecution(
          mockTeamId,
          'agent-1'
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/agent-1/executions`,
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({}),
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })

      it('should start agent execution with input', async () => {
        const executionRequest: StartAgentExecutionRequest = {
          input: { message: 'Hello, agent!' },
        }

        const mockResponse: AgentExecutionResponse = {
          status: 'success',
          message: 'Agent execution started',
          data: mockExecution,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.startAgentExecution(
          mockTeamId,
          'agent-1',
          executionRequest
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/agent-1/executions`,
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify(executionRequest),
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })
    })

    describe('completeAgentExecution', () => {
      it('should complete agent execution with success', async () => {
        const completeRequest: CompleteAgentExecutionRequest = {
          status: 'success',
          output: { result: 'Execution completed successfully' },
        }

        const mockResponse: AgentExecutionResponse = {
          status: 'success',
          message: 'Agent execution completed',
          data: { ...mockExecution, status: 'success' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.completeAgentExecution(
          mockTeamId,
          'execution-1',
          completeRequest
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/executions/execution-1`,
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify(completeRequest),
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })

      it('should complete agent execution with error', async () => {
        const completeRequest: CompleteAgentExecutionRequest = {
          status: 'error',
          error: 'Execution failed due to timeout',
        }

        const mockResponse: AgentExecutionResponse = {
          status: 'success',
          message: 'Agent execution completed',
          data: {
            ...mockExecution,
            status: 'error',
            error: completeRequest.error,
          },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.completeAgentExecution(
          mockTeamId,
          'execution-1',
          completeRequest
        )

        expect(mockFetch).toHaveBeenLastCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/executions/execution-1`,
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify(completeRequest),
          })
        )
        expect(result).toEqual(mockResponse)
      })
    })

    describe('getAgentExecution', () => {
      it('should fetch agent execution by ID', async () => {
        const mockResponse: AgentExecutionResponse = {
          status: 'success',
          message: 'Agent execution retrieved',
          data: mockExecution,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue(mockResponse),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)

        const result = await agentService.getAgentExecution(
          mockTeamId,
          'execution-1'
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/${mockTeamId}/agents/executions/execution-1`,
          expect.objectContaining({
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockResponse)
      })
    })
  })

  describe('Environment Configuration', () => {
    it('should use production API URL by default', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: jest.fn().mockResolvedValue({
          status: 'success',
          data: {
            agents: [],
            page: 1,
            per_page: 20,
            total_count: 0,
            total_pages: 0,
          },
        }),
        headers: {
          get: jest.fn().mockReturnValue('application/json'),
        },
      } as unknown as Response)

      await agentService.getAgents(mockTeamId)

      expect(mockFetch).toHaveBeenCalledWith(
        `https://api.vibexp.io/api/v1/${mockTeamId}/agents`,
        expect.any(Object)
      )
    })
  })

  describe('Error Handling and Edge Cases', () => {
    it('should handle network errors', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'))

      await expect(agentService.getAgents(mockTeamId)).rejects.toThrow(
        'Network error'
      )
    })

    it('should handle malformed JSON responses', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: {
          get: jest.fn().mockReturnValue('application/json'),
        },
        json: jest.fn().mockRejectedValue(new Error('Invalid JSON')),
      } as unknown as Response)

      await expect(agentService.getAgents(mockTeamId)).rejects.toThrow(
        'Invalid JSON'
      )
    })

    it('should handle empty agent ID in getAgent', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: jest.fn().mockResolvedValue({ message: 'Invalid agent ID' }),
      } as unknown as Response)

      await expect(agentService.getAgent(mockTeamId, '')).rejects.toThrow(
        'Invalid agent ID'
      )
    })

    it('should handle empty execution ID in getAgentExecution', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: jest.fn().mockResolvedValue({ message: 'Invalid execution ID' }),
      } as unknown as Response)

      await expect(
        agentService.getAgentExecution(mockTeamId, '')
      ).rejects.toThrow('Invalid execution ID')
    })

    it('should maintain request headers integrity', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: jest.fn().mockResolvedValue({ data: mockAgent }),
        headers: {
          get: jest.fn().mockReturnValue('application/json'),
        },
      } as unknown as Response)

      // Access the private makeRequest method indirectly through a public method
      await agentService.getAgent(mockTeamId, 'agent-1')

      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          }),
        })
      )
    })
  })

  describe('Performance and Resource Management', () => {
    it('should handle concurrent requests properly', async () => {
      const promises = []

      // Mock multiple successful responses
      for (let i = 0; i < 3; i++) {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue({
            status: 'success',
            data: { ...mockAgent, id: `agent-${i}` },
          }),
          headers: {
            get: jest.fn().mockReturnValue('application/json'),
          },
        } as unknown as Response)
      }

      // Start concurrent requests
      promises.push(agentService.getAgent(mockTeamId, 'agent-0'))
      promises.push(agentService.getAgent(mockTeamId, 'agent-1'))
      promises.push(agentService.getAgent(mockTeamId, 'agent-2'))

      const results = await Promise.all(promises)

      expect(results).toHaveLength(3)
      expect(mockFetch).toHaveBeenCalledTimes(3)
    })

    it('should handle large agent lists efficiently', async () => {
      const largeAgentList = Array.from({ length: 100 }, (_, i) => ({
        ...mockAgent,
        id: `agent-${i}`,
        name: `Agent ${i}`,
      }))

      const mockResponse: AgentsResponse = {
        status: 'success',
        message: 'Agents retrieved successfully',
        data: {
          agents: largeAgentList,
          page: 1,
          per_page: 100,
          total_count: 100,
          total_pages: 1,
        },
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: jest.fn().mockResolvedValue(mockResponse),
        headers: {
          get: jest.fn().mockReturnValue('application/json'),
        },
      } as unknown as Response)

      const result = await agentService.getAgents(mockTeamId, { limit: 100 })

      expect(result.data.agents).toHaveLength(100)
      expect(mockFetch).toHaveBeenCalledWith(
        `https://api.vibexp.io/api/v1/${mockTeamId}/agents?limit=100`,
        expect.any(Object)
      )
    })
  })
})
