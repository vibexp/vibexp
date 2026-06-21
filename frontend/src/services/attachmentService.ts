import { apiClient } from '../lib/apiClient'
import type { Attachment, AttachmentListResponse } from '../types/attachment'
import { getApiBaseUrl } from '../utils/environment'

/**
 * Client for the universal attachments API. The backend subsystem is generic
 * (polymorphic owner_type/owner_id); collection operations (list/upload) carry the
 * owner, while item operations (download/remove) are keyed only by the attachment's
 * own id. Attaching files to a new resource type needs no new service code — just
 * pass its ownerType/ownerId.
 */
class AttachmentService {
  private basePath(teamId: string): string {
    return `/${encodeURIComponent(teamId)}/attachments`
  }

  async list(
    teamId: string,
    ownerType: string,
    ownerId: string
  ): Promise<AttachmentListResponse> {
    const query = new URLSearchParams({
      owner_type: ownerType,
      owner_id: ownerId,
    })
    return apiClient.get<AttachmentListResponse>(
      `${this.basePath(teamId)}?${query.toString()}`
    )
  }

  async upload(
    teamId: string,
    ownerType: string,
    ownerId: string,
    file: File
  ): Promise<Attachment> {
    const formData = new FormData()
    formData.append('owner_type', ownerType)
    formData.append('owner_id', ownerId)
    formData.append('file', file)
    // apiClient detects FormData and lets the browser set the multipart
    // Content-Type with its boundary.
    return apiClient.post<Attachment>(this.basePath(teamId), formData)
  }

  async remove(teamId: string, attachmentId: string): Promise<void> {
    await apiClient.delete(
      `${this.basePath(teamId)}/${encodeURIComponent(attachmentId)}`
    )
  }

  /**
   * Fetches an attachment's bytes. The endpoint streams with
   * Content-Disposition: attachment; the caller turns the Blob into a download.
   * Uses fetch directly (not apiClient, which only handles JSON responses).
   */
  async download(teamId: string, attachmentId: string): Promise<Blob> {
    const url = `${getApiBaseUrl()}${this.basePath(teamId)}/${encodeURIComponent(attachmentId)}`
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
