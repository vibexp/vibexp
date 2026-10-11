import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the team statuses domain (#1265) — the OpenAPI spec
// is the single source of truth; do not hand-write request/response shapes here.
export type StatusDefinition = components['schemas']['StatusDefinition']
export type StatusResourceType = components['schemas']['StatusResourceType']
export type StatusListResponse = components['schemas']['StatusListResponse']
export type CreateStatusRequest = components['schemas']['CreateStatusRequest']
export type DeleteStatusResponse = components['schemas']['DeleteStatusResponse']

/**
 * The system status a resource falls back to when the custom status it carries
 * is deleted: `draft` for prompts, `active` for everything else.
 *
 * The server owns the rule and reports the outcome (`reassigned_to` on the
 * delete response), but only after the fact — a confirmation has to name the
 * destination before the request is sent, so this mirrors it. Keep it the one
 * copy on the client.
 */
export const STATUS_DELETE_FALLBACK: Record<StatusResourceType, string> = {
  prompt: 'draft',
  artifact: 'active',
  blueprint: 'active',
  memory: 'active',
}

// statusService talks to the team-scoped /statuses endpoints. Every call takes
// the team id first, mirroring typeService. `unwrap` throws the server's
// `ApiError` untouched, so a caller can show the 409/400 `detail` as written.
class StatusService {
  /** System statuses first, then the team's custom ones, each with its usage. */
  async list(
    teamId: string,
    resourceType: StatusResourceType
  ): Promise<StatusDefinition[]> {
    const response = await unwrap<StatusListResponse>(
      generatedClient.GET('/api/v1/{team_id}/statuses', {
        params: {
          path: { team_id: teamId },
          query: { resource_type: resourceType },
        },
      })
    )
    return response.statuses
  }

  async create(
    teamId: string,
    data: CreateStatusRequest
  ): Promise<StatusDefinition> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/statuses', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    )
  }

  /** Only the display name can change; the slug is immutable. */
  async rename(
    teamId: string,
    id: string,
    name: string
  ): Promise<StatusDefinition> {
    return unwrap(
      generatedClient.PATCH('/api/v1/{team_id}/statuses/{id}', {
        params: { path: { team_id: teamId, id } },
        body: { name },
      })
    )
  }

  /** Deletes a custom status; resources carrying it move to the fallback. */
  async remove(teamId: string, id: string): Promise<DeleteStatusResponse> {
    return unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/statuses/{id}', {
        params: { path: { team_id: teamId, id } },
      })
    )
  }
}

export const statusService = new StatusService()
