import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '@/lib/apiClientGenerated'

// Generated wire types for the notifications domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes here.
export type Notification = components['schemas']['Notification']
export type NotificationListResponse =
  components['schemas']['NotificationListResponse']
export type UnreadCountResponse = components['schemas']['UnreadCountResponse']
export type NotificationListParams = NonNullable<
  operations['listNotifications']['parameters']['query']
>

class NotificationService {
  async listNotifications(
    params: NotificationListParams = {}
  ): Promise<NotificationListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/notifications', {
        params: { query: params },
      })
    )
  }

  async getUnreadCount(): Promise<UnreadCountResponse> {
    return unwrap(generatedClient.GET('/api/v1/notifications/unread-count'))
  }

  async markAsRead(id: string): Promise<void> {
    await unwrap(
      generatedClient.PATCH('/api/v1/notifications/{id}/read', {
        params: { path: { id } },
      })
    )
  }

  async markAllAsRead(): Promise<void> {
    await unwrap(generatedClient.PATCH('/api/v1/notifications/read-all'))
  }
}

export const notificationService = new NotificationService()
