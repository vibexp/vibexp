// Feed types — matching OpenAPI schemas/feeds.yaml

export interface Feed {
  id: string
  team_id: string
  name: string
  description?: string | null
  created_by_user_id: string
  created_at: string
  updated_at: string
}

export interface FeedItem {
  id: string
  team_id: string
  feed_id: string
  project_id?: string | null
  title: string
  content: string
  excerpt: string
  ai_assistant_name: string
  posted_by_user_id: string
  archived_at?: string | null
  posted_at: string
  reply_count?: number
}

export interface FeedItemReply {
  id: string
  team_id: string
  feed_item_id: string
  content: string
  posted_by_user_id: string
  ai_assistant_name?: string | null
  posted_at: string
}

export interface CreateFeedItemReplyRequest {
  content: string
  ai_assistant_name?: string
}

export interface FeedItemReplyListResponse {
  replies: FeedItemReply[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface CreateFeedRequest {
  name: string
  description?: string
}

export interface UpdateFeedRequest {
  name?: string
  description?: string
}

export interface CreateFeedItemRequest {
  title: string
  content: string
  ai_assistant_name: string
  project_id?: string
}

export interface FeedListResponse {
  feeds: Feed[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface FeedItemListResponse {
  items: FeedItem[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface FeedFilters {
  search?: string
  page?: number
  limit?: number
}

export interface FeedItemFilters {
  feed_id?: string
  project_id?: string
  ai_assistant_name?: string
  archived?: 'true' | 'false' | 'all'
  search?: string
  page?: number
  limit?: number
}
