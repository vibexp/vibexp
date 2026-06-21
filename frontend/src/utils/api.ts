import {
  type CursorHooksResponse,
  type CursorOverviewStatsResponse,
  type CursorRecentActivitiesResponse,
  type CursorSessionCountsResponse,
  type CursorSessionsResponse,
  type HooksResponse,
  type OverviewStatsResponse,
  type RecentActivitiesResponse,
  type SessionCountsResponse,
  type SessionsResponse,
} from '../types'
import { getApiBaseUrl } from './environment'

class ApiClient {
  private baseURL: string = getApiBaseUrl().replace('/api/v1', '')

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseURL}${endpoint}`
    const config: RequestInit = {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...(options.headers as Record<string, string>),
      },
      credentials: 'include',
    }

    try {
      const response = await fetch(url, config)

      if (!response.ok) {
        if (response.status === 401) {
          // Session expired — redirect to sign-in
          window.location.href = '/sign-in'
          throw new Error('Session expired. Please sign in again.')
        }
        const errorData = (await response
          .json()
          .catch(() => ({ message: 'Network error' }))) as { message?: string }
        throw new Error(
          errorData.message ??
            `HTTP ${String(response.status)}: ${response.statusText}`
        )
      }

      return (await response.json()) as T
    } catch (error) {
      console.error('API request failed:', error)
      throw error
    }
  }

  // Health check endpoints
  async ping(): Promise<string> {
    const response = await fetch(`${this.baseURL}/ping`, {
      credentials: 'include',
    })
    return response.text()
  }

  async health(): Promise<{ status: string }> {
    const response = await fetch(`${this.baseURL}/health`, {
      credentials: 'include',
    })
    return response.json() as Promise<{ status: string }>
  }

  // Claude Code hooks endpoints
  async getSessions(page = 1, limit = 10): Promise<SessionsResponse> {
    return this.request<SessionsResponse>(
      `/api/v1/ai-tools/claude-code/sessions?page=${String(page)}&limit=${String(limit)}`
    )
  }

  async getHooks(
    page = 1,
    limit = 10,
    sessionId?: string,
    hookEventName?: string,
    toolName?: string
  ): Promise<HooksResponse> {
    const params = new URLSearchParams({
      page: page.toString(),
      limit: limit.toString(),
    })

    if (sessionId) params.append('session_id', sessionId)
    if (hookEventName) params.append('hook_event_name', hookEventName)
    if (toolName) params.append('tool_name', toolName)

    return this.request<HooksResponse>(
      `/api/v1/ai-tools/claude-code/hooks?${params.toString()}`
    )
  }

  async getSessionCounts(range = '7d'): Promise<SessionCountsResponse> {
    return this.request<SessionCountsResponse>(
      `/api/v1/ai-tools/claude-code/session-counts?range=${encodeURIComponent(range)}`
    )
  }

  async getOverviewStats(): Promise<OverviewStatsResponse> {
    return this.request<OverviewStatsResponse>(
      `/api/v1/ai-tools/claude-code/overview-stats`
    )
  }

  async getRecentActivities(): Promise<RecentActivitiesResponse> {
    return this.request<RecentActivitiesResponse>(
      `/api/v1/ai-tools/claude-code/recent-activities`
    )
  }

  async deleteSession(sessionId: string): Promise<void> {
    const url = `${this.baseURL}/api/v1/ai-tools/claude-code/sessions/${encodeURIComponent(sessionId)}`
    const response = await fetch(url, {
      method: 'DELETE',
      credentials: 'include',
    })

    if (!response.ok) {
      if (response.status === 401) {
        window.location.href = '/sign-in'
        throw new Error('Session expired. Please sign in again.')
      }
      if (response.status === 404) {
        throw new Error('Session not found or access denied')
      }
      const errorData = (await response
        .json()
        .catch(() => ({ message: 'Network error' }))) as { message?: string }
      throw new Error(
        errorData.message ??
          `HTTP ${String(response.status)}: ${response.statusText}`
      )
    }
  }

  // Cursor IDE hooks endpoints
  async getCursorSessions(
    page = 1,
    limit = 10
  ): Promise<CursorSessionsResponse> {
    return this.request<CursorSessionsResponse>(
      `/api/v1/ai-tools/cursor-ide/sessions?page=${String(page)}&limit=${String(limit)}`
    )
  }

  async getCursorHooks(
    page = 1,
    limit = 10,
    sessionId?: string,
    hookEventName?: string,
    toolName?: string
  ): Promise<CursorHooksResponse> {
    const params = new URLSearchParams({
      page: page.toString(),
      limit: limit.toString(),
    })

    if (sessionId) params.append('session_id', sessionId)
    if (hookEventName) params.append('hook_event_name', hookEventName)
    if (toolName) params.append('tool_name', toolName)

    return this.request<CursorHooksResponse>(
      `/api/v1/ai-tools/cursor-ide/hooks?${params.toString()}`
    )
  }

  async getCursorSessionCounts(
    range = '7d'
  ): Promise<CursorSessionCountsResponse> {
    return this.request<CursorSessionCountsResponse>(
      `/api/v1/ai-tools/cursor-ide/session-counts?range=${encodeURIComponent(range)}`
    )
  }

  async getCursorOverviewStats(): Promise<CursorOverviewStatsResponse> {
    return this.request<CursorOverviewStatsResponse>(
      `/api/v1/ai-tools/cursor-ide/overview-stats`
    )
  }

  async getCursorRecentActivities(): Promise<CursorRecentActivitiesResponse> {
    return this.request<CursorRecentActivitiesResponse>(
      `/api/v1/ai-tools/cursor-ide/recent-activities`
    )
  }

  async deleteCursorSession(sessionId: string): Promise<void> {
    const url = `${this.baseURL}/api/v1/ai-tools/cursor-ide/sessions/${encodeURIComponent(sessionId)}`
    const response = await fetch(url, {
      method: 'DELETE',
      credentials: 'include',
    })

    if (!response.ok) {
      if (response.status === 401) {
        window.location.href = '/sign-in'
        throw new Error('Session expired. Please sign in again.')
      }
      if (response.status === 404) {
        throw new Error('Session not found or access denied')
      }
      const errorData = (await response
        .json()
        .catch(() => ({ message: 'Network error' }))) as { message?: string }
      throw new Error(
        errorData.message ??
          `HTTP ${String(response.status)}: ${response.statusText}`
      )
    }
  }

  // Test connection method
  async testConnection(): Promise<boolean> {
    try {
      await this.health()
      return true
    } catch {
      return false
    }
  }
}

export const apiClient = new ApiClient()
