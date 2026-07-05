import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import { getApiBaseUrl } from '../utils/environment'

// Generated wire types for the attachments domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes here.
export type Attachment = components['schemas']['Attachment']
export type AttachmentListResponse =
  components['schemas']['AttachmentListResponse']

/**
 * Client for the universal attachments API. The backend subsystem is generic
 * (polymorphic owner_type/owner_id); collection operations (list/upload) carry the
 * owner, while item operations (download/remove) are keyed only by the attachment's
 * own id. Attaching files to a new resource type needs no new service code — just
 * pass its ownerType/ownerId.
 */
class AttachmentService {
  async list(
    teamId: string,
    ownerType: string,
    ownerId: string
  ): Promise<AttachmentListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/attachments', {
        params: {
          path: { team_id: teamId },
          query: { owner_type: ownerType, owner_id: ownerId },
        },
      })
    )
  }

  async upload(
    teamId: string,
    ownerType: string,
    ownerId: string,
    file: File
  ): Promise<Attachment> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/attachments', {
        params: { path: { team_id: teamId } },
        // The spec types the binary part as string; the serializer below sends
        // the actual File. openapi-fetch omits its default JSON Content-Type
        // for FormData bodies, so the browser sets multipart with a boundary.
        body: {
          owner_type: ownerType,
          owner_id: ownerId,
          file: file as unknown as string,
        },
        bodySerializer: body => {
          const formData = new FormData()
          formData.append('owner_type', body.owner_type)
          formData.append('owner_id', body.owner_id)
          formData.append('file', file)
          return formData
        },
      })
    )
  }

  async remove(teamId: string, attachmentId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/attachments/{id}', {
        params: { path: { team_id: teamId, id: attachmentId } },
      })
    )
  }

  /**
   * Fetches an attachment's bytes. The endpoint streams application/octet-stream
   * with Content-Disposition: attachment; the caller turns the Blob into a
   * download. Stays a thin fetch wrapper: unwrap() resolves JSON payloads, and
   * routing a binary response through the generated client would need a
   * parseAs-aware error path for no practical gain.
   */
  async download(teamId: string, attachmentId: string): Promise<Blob> {
    const url = `${getApiBaseUrl()}/${encodeURIComponent(teamId)}/attachments/${encodeURIComponent(attachmentId)}`
    const response = await fetch(url, { credentials: 'include' })
    if (!response.ok) {
      throw new Error(
        `Failed to download attachment (HTTP ${String(response.status)})`
      )
    }
    return response.blob()
  }
}

export const attachmentService = new AttachmentService()
