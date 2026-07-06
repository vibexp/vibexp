import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the feeds domain — the OpenAPI spec is the single
// source of truth; do not hand-write request/response shapes here.
export type Feed = components['schemas']['Feed']
export type FeedItem = components['schemas']['FeedItem']
export type FeedItemReply = components['schemas']['FeedItemReply']
export type CreateFeedItemReplyRequest =
  components['schemas']['CreateFeedItemReplyRequest']
export type FeedItemReplyListResponse =
  components['schemas']['FeedItemReplyListResponse']
export type CreateFeedRequest = components['schemas']['CreateFeedRequest']
export type UpdateFeedRequest = components['schemas']['UpdateFeedRequest']
export type CreateFeedItemRequest =
  components['schemas']['CreateFeedItemRequest']
export type FeedListResponse = components['schemas']['FeedListResponse']
export type FeedItemListResponse = components['schemas']['FeedItemListResponse']

export type FeedFilters = NonNullable<
  operations['listFeeds']['parameters']['query']
>

// The generated feed-item list queries omit `search` (and `ai_assistant_name`
// on the by-feed variant), which the UI still forwards, so the richer
// hand-written filter shape is kept and serialized directly.
export interface FeedItemFilters {
  feed_id?: string
  project_id?: string
  ai_assistant_name?: string
  archived?: 'true' | 'false' | 'all'
  search?: string
  page?: number
  limit?: number
}

class FeedService {
  async getFeeds(
    teamId: string,
    filters: FeedFilters = {}
  ): Promise<FeedListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/feeds', {
        params: { path: { team_id: teamId }, query: filters },
      })
    )
  }

  async getFeed(teamId: string, feedId: string): Promise<Feed> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/feeds/{feed_id}', {
        params: { path: { team_id: teamId, feed_id: feedId } },
      })
    )
  }

  async createFeed(teamId: string, body: CreateFeedRequest): Promise<Feed> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/feeds', {
        params: { path: { team_id: teamId } },
        body,
      })
    )
  }

  async updateFeed(
    teamId: string,
    feedId: string,
    body: UpdateFeedRequest
  ): Promise<Feed> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/feeds/{feed_id}', {
        params: { path: { team_id: teamId, feed_id: feedId } },
        body,
      })
    )
  }

  async deleteFeed(teamId: string, feedId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/feeds/{feed_id}', {
        params: { path: { team_id: teamId, feed_id: feedId } },
      })
    )
  }

  async getFeedItems(
    teamId: string,
    filters: FeedItemFilters = {}
  ): Promise<FeedItemListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/feed-items', {
        params: {
          path: { team_id: teamId },
          query: filters,
        },
      })
    )
  }

  async getFeedItemsForFeed(
    teamId: string,
    feedId: string,
    filters: FeedItemFilters = {}
  ): Promise<FeedItemListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/feeds/{feed_id}/items', {
        params: {
          path: { team_id: teamId, feed_id: feedId },
          query: filters,
        },
      })
    )
  }

  async getFeedItem(teamId: string, itemId: string): Promise<FeedItem> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/feed-items/{item_id}', {
        params: { path: { team_id: teamId, item_id: itemId } },
      })
    )
  }

  async createFeedItem(
    teamId: string,
    feedId: string,
    body: CreateFeedItemRequest
  ): Promise<FeedItem> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/feeds/{feed_id}/items', {
        params: { path: { team_id: teamId, feed_id: feedId } },
        body,
      })
    )
  }

  async archiveFeedItem(teamId: string, itemId: string): Promise<void> {
    await unwrap(
      generatedClient.POST('/api/v1/{team_id}/feed-items/{item_id}/archive', {
        params: { path: { team_id: teamId, item_id: itemId } },
      })
    )
  }

  async unarchiveFeedItem(teamId: string, itemId: string): Promise<void> {
    await unwrap(
      generatedClient.POST('/api/v1/{team_id}/feed-items/{item_id}/unarchive', {
        params: { path: { team_id: teamId, item_id: itemId } },
      })
    )
  }

  async deleteFeedItem(teamId: string, itemId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/feed-items/{item_id}', {
        params: { path: { team_id: teamId, item_id: itemId } },
      })
    )
  }

  async listReplies(
    teamId: string,
    itemId: string,
    page?: number,
    limit?: number
  ): Promise<FeedItemReplyListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/feed-items/{item_id}/replies', {
        params: {
          path: { team_id: teamId, item_id: itemId },
          query: { page, limit },
        },
      })
    )
  }

  async createReply(
    teamId: string,
    itemId: string,
    body: CreateFeedItemReplyRequest
  ): Promise<FeedItemReply> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/feed-items/{item_id}/replies', {
        params: { path: { team_id: teamId, item_id: itemId } },
        body,
      })
    )
  }
}

export const feedService = new FeedService()
