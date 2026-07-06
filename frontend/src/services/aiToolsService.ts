import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '@/lib/apiClientGenerated'

// Generated wire types for the AI-tools (Claude Code / Cursor IDE) domain — the
// OpenAPI spec is the single source of truth; do not hand-write response shapes
// here. The endpoints wrap their payload in a `{ status, message, data }`
// envelope, so each method unwraps to the inner `data` (see the envelope rule in
// docs/developer-guidelines/frontend/api-integration.md).
export type OverviewStats = components['schemas']['OverviewStats']
export type CursorOverviewStats = components['schemas']['CursorOverviewStats']
export type SessionCounts = components['schemas']['SessionCountsResponse']
export type RecentActivities = components['schemas']['RecentActivitiesResponse']
export type CursorRecentActivities =
  components['schemas']['CursorRecentActivitiesResponse']
export type RecentActivity = components['schemas']['RecentActivity']
export type CursorRecentActivity = components['schemas']['CursorRecentActivity']

/**
 * AI-tools metrics service backed by `GET /api/v1/ai-tools/{claude-code,cursor-ide}/*`.
 *
 * Replaces the parallel hand-written `utils/api.ts` client so page components
 * depend on a service (like `notificationService`) instead of doing HTTP
 * themselves. Authentication is the httpOnly session cookie sent by
 * `generatedClient` (`credentials: 'include'`).
 */
class AIToolsService {
  async getClaudeCodeSessionCounts(range = '7d'): Promise<SessionCounts> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/ai-tools/claude-code/session-counts', {
          params: { query: { range } },
        })
      )
    ).data
  }

  async getClaudeCodeOverviewStats(): Promise<OverviewStats> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/ai-tools/claude-code/overview-stats')
      )
    ).data
  }

  async getClaudeCodeRecentActivities(): Promise<RecentActivities> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/ai-tools/claude-code/recent-activities')
      )
    ).data
  }

  async getCursorIDESessionCounts(range = '7d'): Promise<SessionCounts> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/ai-tools/cursor-ide/session-counts', {
          params: { query: { range } },
        })
      )
    ).data
  }

  async getCursorIDEOverviewStats(): Promise<CursorOverviewStats> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/ai-tools/cursor-ide/overview-stats')
      )
    ).data
  }

  async getCursorIDERecentActivities(): Promise<CursorRecentActivities> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/ai-tools/cursor-ide/recent-activities')
      )
    ).data
  }
}

export const aiToolsService = new AIToolsService()
