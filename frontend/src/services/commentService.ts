import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the comments domain — the OpenAPI spec is the single
// source of truth; do not hand-write request/response shapes here.
export type Comment = components['schemas']['Comment']
export type CreateCommentRequest = components['schemas']['CreateCommentRequest']
export type UpdateCommentRequest = components['schemas']['UpdateCommentRequest']
export type CommentListResponse = components['schemas']['CommentListResponse']
export type RecentComment = components['schemas']['RecentComment']
export type RecentCommentListResponse =
  components['schemas']['RecentCommentListResponse']

/**
 * The four resource kinds a comment can attach to. The wire type is a bare
 * `string`, but every call site in the SPA passes one of these literals, so we
 * narrow it here for type-safety at the widget boundary.
 */
export type CommentResourceType = 'artifact' | 'memory' | 'prompt' | 'blueprint'

/**
 * Thin wrapper over the generated client for resource comments (feedService-
 * shaped). Every method resolves through `unwrap` so failures throw the same
 * `ApiError` the rest of the SPA handles.
 */
class CommentService {
  async list(
    teamId: string,
    resourceType: CommentResourceType,
    resourceId: string,
    page?: number,
    limit?: number
  ): Promise<CommentListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/comments', {
        params: {
          path: { team_id: teamId },
          query: {
            resource_type: resourceType,
            resource_id: resourceId,
            page,
            limit,
          },
        },
      })
    )
  }

  async create(teamId: string, body: CreateCommentRequest): Promise<Comment> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/comments', {
        params: { path: { team_id: teamId } },
        body,
      })
    )
  }

  async update(
    teamId: string,
    commentId: string,
    body: UpdateCommentRequest
  ): Promise<Comment> {
    return unwrap(
      generatedClient.PATCH('/api/v1/{team_id}/comments/{comment_id}', {
        params: { path: { team_id: teamId, comment_id: commentId } },
        body,
      })
    )
  }

  async remove(teamId: string, commentId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/comments/{comment_id}', {
        params: { path: { team_id: teamId, comment_id: commentId } },
      })
    )
  }

  /**
   * Team-wide recent comment activity (latest state per comment) for the
   * homepage card. Rows carry the resource's resolved title + link fields, so
   * links never go stale; the server omits rows whose resource has vanished.
   */
  async recent(
    teamId: string,
    limit?: number
  ): Promise<RecentCommentListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/comments/recent', {
        params: { path: { team_id: teamId }, query: { limit } },
      })
    )
  }
}

export const commentService = new CommentService()
