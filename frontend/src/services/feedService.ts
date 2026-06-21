import { apiClient } from '../lib/apiClient'
import type {
  CreateFeedItemReplyRequest,
  CreateFeedItemRequest,
  CreateFeedRequest,
  Feed,
  FeedFilters,
  FeedItem,
  FeedItemFilters,
  FeedItemListResponse,
  FeedItemReply,
  FeedItemReplyListResponse,
  FeedListResponse,
  UpdateFeedRequest,
} from '../types/feed'

class FeedService {
  async getFeeds(
    teamId: string,
    filters: FeedFilters = {}
  ): Promise<FeedListResponse> {
    const params = new URLSearchParams()
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined) {
        params.append(key, String(value))
      }
    })
    const queryString = params.toString()
    const endpoint = `/${encodeURIComponent(teamId)}/feeds${queryString ? `?${queryString}` : ''}`
    return apiClient.get<FeedListResponse>(endpoint)
  }

  async getFeed(teamId: string, feedId: string): Promise<Feed> {
    return apiClient.get<Feed>(
      `/${encodeURIComponent(teamId)}/feeds/${encodeURIComponent(feedId)}`
    )
  }

  async createFeed(teamId: string, body: CreateFeedRequest): Promise<Feed> {
    return apiClient.post<Feed>(`/${encodeURIComponent(teamId)}/feeds`, body)
  }

  async updateFeed(
    teamId: string,
    feedId: string,
    body: UpdateFeedRequest
  ): Promise<Feed> {
    return apiClient.put<Feed>(
      `/${encodeURIComponent(teamId)}/feeds/${encodeURIComponent(feedId)}`,
      body
    )
  }

  async deleteFeed(teamId: string, feedId: string): Promise<void> {
    await apiClient.delete(
      `/${encodeURIComponent(teamId)}/feeds/${encodeURIComponent(feedId)}`
    )
  }

  async getFeedItems(
    teamId: string,
    filters: FeedItemFilters = {}
  ): Promise<FeedItemListResponse> {
    const params = new URLSearchParams()
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined) {
        params.append(key, String(value))
      }
    })
    const queryString = params.toString()
    const endpoint = `/${encodeURIComponent(teamId)}/feed-items${queryString ? `?${queryString}` : ''}`
    return apiClient.get<FeedItemListResponse>(endpoint)
  }

  async getFeedItemsForFeed(
    teamId: string,
    feedId: string,
    filters: FeedItemFilters = {}
  ): Promise<FeedItemListResponse> {
    const params = new URLSearchParams()
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined) {
        params.append(key, String(value))
      }
    })
    const queryString = params.toString()
    const endpoint = `/${encodeURIComponent(teamId)}/feeds/${encodeURIComponent(feedId)}/items${queryString ? `?${queryString}` : ''}`
    return apiClient.get<FeedItemListResponse>(endpoint)
  }

  async getFeedItem(teamId: string, itemId: string): Promise<FeedItem> {
    return apiClient.get<FeedItem>(
      `/${encodeURIComponent(teamId)}/feed-items/${encodeURIComponent(itemId)}`
    )
  }

  async createFeedItem(
    teamId: string,
    feedId: string,
    body: CreateFeedItemRequest
  ): Promise<FeedItem> {
    return apiClient.post<FeedItem>(
      `/${encodeURIComponent(teamId)}/feeds/${encodeURIComponent(feedId)}/items`,
      body
    )
  }

  async archiveFeedItem(teamId: string, itemId: string): Promise<void> {
    await apiClient.post(
      `/${encodeURIComponent(teamId)}/feed-items/${encodeURIComponent(itemId)}/archive`
    )
  }

  async unarchiveFeedItem(teamId: string, itemId: string): Promise<void> {
    await apiClient.post(
      `/${encodeURIComponent(teamId)}/feed-items/${encodeURIComponent(itemId)}/unarchive`
    )
  }

  async deleteFeedItem(teamId: string, itemId: string): Promise<void> {
    await apiClient.delete(
      `/${encodeURIComponent(teamId)}/feed-items/${encodeURIComponent(itemId)}`
    )
  }

  async listReplies(
    teamId: string,
    itemId: string,
    page?: number,
    limit?: number
  ): Promise<FeedItemReplyListResponse> {
    const params = new URLSearchParams()
    if (page !== undefined) params.append('page', String(page))
    if (limit !== undefined) params.append('limit', String(limit))
    const queryString = params.toString()
    const endpoint = `/${encodeURIComponent(teamId)}/feed-items/${encodeURIComponent(itemId)}/replies${queryString ? `?${queryString}` : ''}`
    return apiClient.get<FeedItemReplyListResponse>(endpoint)
  }

  async createReply(
    teamId: string,
    itemId: string,
    body: CreateFeedItemReplyRequest
  ): Promise<FeedItemReply> {
    return apiClient.post<FeedItemReply>(
      `/${encodeURIComponent(teamId)}/feed-items/${encodeURIComponent(itemId)}/replies`,
      body
    )
  }
}

export const feedService = new FeedService()
