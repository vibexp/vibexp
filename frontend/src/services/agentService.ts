import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the agent domain (epic #87) — the OpenAPI spec is the
// single source of truth. AgentSkill/AgentCapabilities/AgentProvider/AgentActivity
// are inlined in their parent schemas, so they are re-derived as indexed-access
// aliases. The A2A `AgentCard` has every field optional/nullable in the spec.
export type AgentCard = components['schemas']['AgentCard']
export type AgentSkill = NonNullable<AgentCard['skills']>[number]
export type AgentCapabilities = NonNullable<AgentCard['capabilities']>
export type AgentProvider = NonNullable<AgentCard['provider']>
export type Agent = components['schemas']['Agent']
export type CreateAgentRequest = components['schemas']['CreateAgentRequest']
export type UpdateAgentRequest = components['schemas']['UpdateAgentRequest']
export type AgentListResponse = components['schemas']['AgentListResponse']
export type AgentStatsResponse = components['schemas']['AgentStatsResponse']
export type AgentActivity = NonNullable<
  AgentStatsResponse['recent_activities']
>[number]
export type AgentExecution = components['schemas']['AgentExecution']
export type AgentExecutionListResponse =
  components['schemas']['AgentExecutionListResponse']
export type ConversationExecutionsResponse =
  components['schemas']['ConversationExecutionsResponse']
export type ConversationSummary = components['schemas']['ConversationSummary']
export type ConversationListResponse =
  components['schemas']['ConversationListResponse']

// Local-only shape with no generated counterpart: a query-param bag for listing.
export interface AgentFilters {
  status?: 'active' | 'paused' | 'error'
  search?: string
  page?: number
  limit?: number
  sort_by?:
    | 'name'
    | 'status'
    | 'total_runs'
    | 'success_rate'
    | 'last_run'
    | 'created_at'
  sort_order?: 'asc' | 'desc'
}

class AgentService {
  async getAgents(
    teamId: string,
    filters: AgentFilters = {}
  ): Promise<AgentListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/agents', {
        params: {
          path: { team_id: teamId },
          query: {
            status: filters.status,
            search: filters.search,
            page: filters.page,
            limit: filters.limit,
          },
        },
      })
    )
  }

  async getAgent(teamId: string, id: string): Promise<Agent> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/agents/{id}', {
        params: { path: { team_id: teamId, id } },
      })
    )
  }

  async createAgent(teamId: string, data: CreateAgentRequest): Promise<Agent> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/agents', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    )
  }

  async updateAgent(
    teamId: string,
    id: string,
    data: UpdateAgentRequest
  ): Promise<Agent> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/agents/{id}', {
        params: { path: { team_id: teamId, id } },
        body: data,
      })
    )
  }

  async deleteAgent(teamId: string, id: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/agents/{id}', {
        params: { path: { team_id: teamId, id } },
      })
    )
  }

  async getAgentStats(teamId: string): Promise<AgentStatsResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/agents/stats', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  async previewAgentCard(teamId: string, cardUrl: string): Promise<AgentCard> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/agents/preview-card', {
        params: { path: { team_id: teamId } },
        body: { card_url: cardUrl },
      })
    )
  }

  async updateAgentCredentials(
    teamId: string,
    agentId: string,
    credentials: Record<string, { type: string; value: string }>
  ): Promise<void> {
    await unwrap(
      generatedClient.PUT('/api/v1/{team_id}/agents/{id}/credentials', {
        params: { path: { team_id: teamId, id: agentId } },
        // The service accepts loosely-typed credential objects; the backend
        // validates the credential `type` enum, so cast to the generated body.
        body: {
          credentials,
        } as components['schemas']['UpdateAgentCredentialsRequest'],
      })
    )
  }

  async executeAgent(
    teamId: string,
    agentId: string,
    input: Record<string, unknown>,
    conversationId?: string
  ): Promise<AgentExecution> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/agents/{id}/execute', {
        params: { path: { team_id: teamId, id: agentId } },
        body: { input, conversation_id: conversationId },
      })
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
  ): Promise<AgentExecutionListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/agents/{id}/executions', {
        params: {
          path: { team_id: teamId, id: agentId },
          query: {
            status: filters?.status,
            date_from: filters?.date_from,
            date_to: filters?.date_to,
            page: filters?.page,
            limit: filters?.limit,
          },
        },
      })
    )
  }

  async getExecutionStatus(
    teamId: string,
    executionId: string
  ): Promise<AgentExecution> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/agents/executions/{id}/status', {
        params: { path: { team_id: teamId, id: executionId } },
      })
    )
  }

  async cancelExecution(
    teamId: string,
    executionId: string
  ): Promise<AgentExecution> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/agents/executions/{execution_id}/cancel',
        {
          params: { path: { team_id: teamId, execution_id: executionId } },
        }
      )
    )
  }

  async getConversationExecutions(
    teamId: string,
    conversationId: string,
    options?: {
      limit?: number
      before?: string // ISO timestamp
    }
  ): Promise<ConversationExecutionsResponse> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/agents/conversations/{conversation_id}/executions',
        {
          params: {
            path: { team_id: teamId, conversation_id: conversationId },
            query: { limit: options?.limit, before: options?.before },
          },
        }
      )
    )
  }

  async listAgentConversations(
    teamId: string,
    agentId: string,
    options?: {
      page?: number
      limit?: number
    }
  ): Promise<ConversationListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/agents/{id}/conversations', {
        params: {
          path: { team_id: teamId, id: agentId },
          query: { page: options?.page, limit: options?.limit },
        },
      })
    )
  }
}

export const agentService = new AgentService()
