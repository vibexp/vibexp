import { apiClient } from '../lib/apiClient'
import type {
  AgentExecutionResponse,
  AgentFilters,
  AgentResponse,
  AgentsResponse,
  AgentStatsApiResponse,
  CompleteAgentExecutionRequest,
  CreateAgentRequest,
  StartAgentExecutionRequest,
  UpdateAgentRequest,
} from '../types'

class AgentService {
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

    return apiClient.get<AgentsResponse>(endpoint)
  }

  async getAgent(teamId: string, id: string): Promise<AgentResponse> {
    return apiClient.get<AgentResponse>(`/${teamId}/agents/${id}`)
  }

  async createAgent(
    teamId: string,
    data: CreateAgentRequest
  ): Promise<AgentResponse> {
    return apiClient.post<AgentResponse>(`/${teamId}/agents`, data)
  }

  async updateAgent(
    teamId: string,
    id: string,
    data: UpdateAgentRequest
  ): Promise<AgentResponse> {
    return apiClient.put<AgentResponse>(`/${teamId}/agents/${id}`, data)
  }

  async deleteAgent(teamId: string, id: string): Promise<void> {
    await apiClient.delete(`/${teamId}/agents/${id}`)
  }

  async getAgentStats(teamId: string): Promise<AgentStatsApiResponse> {
    return apiClient.get<AgentStatsApiResponse>(`/${teamId}/agents/stats`)
  }

  async startAgentExecution(
    teamId: string,
    agentId: string,
    data: StartAgentExecutionRequest = {}
  ): Promise<AgentExecutionResponse> {
    return apiClient.post<AgentExecutionResponse>(
      `/${teamId}/agents/${agentId}/executions`,
      data
    )
  }

  async completeAgentExecution(
    teamId: string,
    executionId: string,
    data: CompleteAgentExecutionRequest
  ): Promise<AgentExecutionResponse> {
    return apiClient.put<AgentExecutionResponse>(
      `/${teamId}/agents/executions/${executionId}`,
      data
    )
  }

  async getAgentExecution(
    teamId: string,
    executionId: string
  ): Promise<AgentExecutionResponse> {
    return apiClient.get<AgentExecutionResponse>(
      `/${teamId}/agents/executions/${executionId}`
    )
  }

  async previewAgentCard(
    teamId: string,
    cardUrl: string
  ): Promise<import('../types').AgentCard> {
    return apiClient.post<import('../types').AgentCard>(
      `/${teamId}/agents/preview-card`,
      {
        card_url: cardUrl,
      }
    )
  }

  async updateAgentCredentials(
    teamId: string,
    agentId: string,
    credentials: Record<string, { type: string; value: string }>
  ): Promise<void> {
    await apiClient.put(`/${teamId}/agents/${agentId}/credentials`, {
      credentials,
    })
  }

  async executeAgent(
    teamId: string,
    agentId: string,
    input: Record<string, unknown>,
    conversationId?: string
  ): Promise<AgentExecutionResponse> {
    return apiClient.post<AgentExecutionResponse>(
      `/${teamId}/agents/${agentId}/execute`,
      {
        input,
        conversation_id: conversationId,
      }
    )
  }

  async listAgentExecutions(
    teamId: string,
    agentId: string,
    filters?: {
      status?: string
      date_from?: string
      date_to?: string
      page?: number
      limit?: number
    }
  ): Promise<{
    executions: import('../types').AgentExecution[]
    total_count: number
    page: number
    per_page: number
    total_pages: number
  }> {
    const params = new URLSearchParams()

    // Remove team_id from query params - it's now in the URL path
    if (filters?.status) params.append('status', filters.status)
    if (filters?.date_from) params.append('date_from', filters.date_from)
    if (filters?.date_to) params.append('date_to', filters.date_to)
    if (filters?.page) params.append('page', filters.page.toString())
    if (filters?.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/${teamId}/agents/${agentId}/executions${queryString ? `?${queryString}` : ''}`

    return apiClient.get(endpoint)
  }

  async getExecutionStatus(
    teamId: string,
    executionId: string
  ): Promise<AgentExecutionResponse> {
    return apiClient.get<AgentExecutionResponse>(
      `/${teamId}/agents/executions/${executionId}/status`
    )
  }

  async getExecutionEvents(
    teamId: string,
    executionId: string,
    filters?: { page?: number; limit?: number }
  ): Promise<import('../types').AgentExecutionEventsResponse> {
    const params = new URLSearchParams()
    if (filters?.page) params.append('page', filters.page.toString())
    if (filters?.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/${teamId}/agents/executions/${executionId}/events${queryString ? `?${queryString}` : ''}`

    return apiClient.get<import('../types').AgentExecutionEventsResponse>(
      endpoint
    )
  }

  async getConversationExecutions(
    teamId: string,
    conversationId: string,
    options?: {
      limit?: number
      before?: string // ISO timestamp
    }
  ): Promise<import('../types').ConversationExecutionsResponse> {
    const params = new URLSearchParams()
    if (options?.limit) params.append('limit', options.limit.toString())
    if (options?.before) params.append('before', options.before)

    const queryString = params.toString()
    const endpoint = `/${teamId}/agents/conversations/${conversationId}/executions${
      queryString ? `?${queryString}` : ''
    }`

    return apiClient.get<import('../types').ConversationExecutionsResponse>(
      endpoint
    )
  }

  async listAgentConversations(
    teamId: string,
    agentId: string,
    options?: {
      page?: number
      limit?: number
    }
  ): Promise<import('../types').ConversationListResponse> {
    const params = new URLSearchParams()
    // Remove team_id from query params - it's now in the URL path
    if (options?.page) params.append('page', options.page.toString())
    if (options?.limit) params.append('limit', options.limit.toString())

    const queryString = params.toString()
    const endpoint = `/${teamId}/agents/${agentId}/conversations${queryString ? `?${queryString}` : ''}`

    return apiClient.get<import('../types').ConversationListResponse>(endpoint)
  }
}

export const agentService = new AgentService()
