import type { ApiResponse } from './api'

// Agent Card types (A2A specification)
export interface AgentSkill {
  id: string
  name: string
  description: string
  tags: string[]
  examples: string[]
  inputModes?: string[]
  outputModes?: string[]
  security?: string
}

export interface AgentCapabilities {
  streaming: boolean
  pushNotifications?: { enabled: boolean }
  stateTransitionHistory?: { enabled: boolean }
  extensions?: { enabled: boolean }
}

export interface AgentProvider {
  name: string
  url: string
}

export interface AgentCard {
  name: string
  description: string
  version: string
  protocolVersion: string
  url: string
  preferredTransport: string
  defaultInputModes: string[]
  defaultOutputModes: string[]
  iconUrl?: string
  documentationUrl?: string
  provider?: AgentProvider
  capabilities?: AgentCapabilities
  skills: AgentSkill[]
  security?: { authenticationRequired: boolean }
  securitySchemes?: Record<string, unknown>
  additionalInterfaces?: Record<string, unknown>
  supportsAuthenticatedExtendedCard?: boolean
  signatures?: Record<string, unknown>
}

// Agent types
export interface Agent {
  id: string
  user_id: string
  team_id: string
  name: string
  description: string
  status: 'active' | 'paused' | 'error'
  card_url?: string
  agent_card?: AgentCard
  config: Record<string, unknown>
  has_credentials?: string[] // List of credential names that are already set
  last_run?: string | null
  last_synced_at?: string | null
  total_runs: number
  success_rate: number
  created_at: string
  updated_at: string
}

export interface CreateAgentRequest {
  card_url: string
  status?: 'active' | 'paused'
}

export interface UpdateAgentRequest {
  card_url?: string
  status?: 'active' | 'paused' | 'error'
}

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

export interface AgentListResponse {
  agents: Agent[]
  page: number
  per_page: number
  total_count: number
  total_pages: number
}

export interface AgentStatsResponse {
  total_agents: number
  active_agents: number
  paused_agents: number
  error_agents: number
  total_runs: number
  avg_success_rate: number
  runs_today: number
  runs_this_week: number
  recent_activities: AgentActivity[]
}

export interface AgentActivity {
  id: string
  agent_id: string
  agent_name: string
  action: string
  status: 'success' | 'warning' | 'error'
  description: string
  created_at: string
}

export interface AgentExecution {
  id: string
  agent_id: string
  user_id: string

  // Update status to include A2A states
  status:
    | 'pending'
    | 'running'
    | 'success'
    | 'error'
    | 'submitted'
    | 'working'
    | 'completed'
    | 'failed'
    | 'cancelled'

  input?: Record<string, unknown> | null
  output?: Record<string, unknown> | null
  error?: string | null
  started_at: string
  ended_at?: string | null
  duration?: number | null

  // A2A streaming fields (optional)
  task_id?: string | null
  context_id?: string | null
  current_state?: string | null
  artifacts?: Record<string, unknown>[] | null

  // Conversation support
  conversation_id?: string | null
}

export interface StartAgentExecutionRequest {
  input?: Record<string, unknown>
  conversation_id?: string // Optional: to continue existing conversation
}

export interface CompleteAgentExecutionRequest {
  status:
    | 'pending'
    | 'running'
    | 'success'
    | 'error'
    | 'submitted'
    | 'working'
    | 'completed'
    | 'failed'
    | 'cancelled'
  output?: Record<string, unknown>
  error?: string
}

export type AgentsResponse = ApiResponse<AgentListResponse>
export type AgentResponse = ApiResponse<Agent>
export type AgentStatsApiResponse = ApiResponse<AgentStatsResponse>
export type AgentExecutionResponse = ApiResponse<AgentExecution>

// Agent Execution Events - A2A Streaming Support
export interface AgentExecutionEvent {
  id: string
  execution_id: string
  event_type: 'task' | 'status-update' | 'artifact-update'
  event_data: Record<string, unknown>
  sequence_number: number
  received_at: string
}

export interface AgentExecutionEventsResponse {
  events: AgentExecutionEvent[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface ConversationExecutionsResponse {
  executions: AgentExecution[]
  conversation_id: string
  count: number
  has_more: boolean
  total_count: number
}

export interface ConversationSummary {
  conversation_id: string
  agent_id: string
  message_count: number
  first_message: string
  last_message: string
  started_at: string
  last_activity_at: string
  last_status: string
}

export interface ConversationListResponse {
  conversations: ConversationSummary[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}
