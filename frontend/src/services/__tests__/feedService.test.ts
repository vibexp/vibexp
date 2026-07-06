import type {
  Feed,
  FeedItem,
  FeedItemListResponse,
  FeedItemReply,
  FeedItemReplyListResponse,
  FeedListResponse,
} from '../feedService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  PUT: jest.fn(),
  DELETE: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { feedService } from '../feedService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const TEAM_ID = 'team-123'
const FEED_ID = 'feed-abc'
const ITEM_ID = 'item-xyz'

const mockFeed: Feed = {
  id: FEED_ID,
  team_id: TEAM_ID,
  name: 'Product Updates',
  description: 'AI-generated updates',
  created_by_user_id: 'user-1',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const mockFeedItem: FeedItem = {
  id: ITEM_ID,
  team_id: TEAM_ID,
  feed_id: FEED_ID,
  title: 'Sprint Retrospective',
  content: '## Summary\nAll tasks completed.',
  excerpt: 'All tasks completed.',
  ai_assistant_name: 'claude-sonnet-4-5',
  posted_by_user_id: 'user-1',
  posted_at: '2024-01-15T10:00:00Z',
  reply_count: 0,
}

const mockReply: FeedItemReply = {
  id: 'reply-1',
  team_id: TEAM_ID,
  feed_item_id: ITEM_ID,
  content: 'Nice work',
  posted_by_user_id: 'user-1',
  posted_at: '2024-01-15T11:00:00Z',
}

describe('FeedService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getFeeds', () => {
    it('returns feed list response', async () => {
      const mockResponse: FeedListResponse = {
        feeds: [mockFeed],
        total_count: 1,
        page: 1,
        per_page: 20,
        total_pages: 1,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await feedService.getFeeds(TEAM_ID)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feeds',
        { params: { path: { team_id: TEAM_ID }, query: {} } }
      )
      expect(result.feeds).toHaveLength(1)
      expect(result.feeds[0].name).toBe('Product Updates')
    })

    it('passes filters as query params', async () => {
      const mockResponse: FeedListResponse = {
        feeds: [],
        total_count: 0,
        page: 2,
        per_page: 10,
        total_pages: 0,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      await feedService.getFeeds(TEAM_ID, { page: 2, limit: 10 })

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feeds',
        {
          params: { path: { team_id: TEAM_ID }, query: { page: 2, limit: 10 } },
        }
      )
    })
  })

  describe('getFeed', () => {
    it('returns a single feed', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockFeed))

      const result = await feedService.getFeed(TEAM_ID, FEED_ID)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feeds/{feed_id}',
        { params: { path: { team_id: TEAM_ID, feed_id: FEED_ID } } }
      )
      expect(result.id).toBe(FEED_ID)
    })
  })

  describe('createFeed', () => {
    it('creates a feed and returns it', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockFeed))

      const body = {
        name: 'Product Updates',
        description: 'AI-generated updates',
      }
      const result = await feedService.createFeed(TEAM_ID, body)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feeds',
        { params: { path: { team_id: TEAM_ID } }, body }
      )
      expect(result.name).toBe('Product Updates')
    })
  })

  describe('updateFeed', () => {
    it('updates a feed and returns it', async () => {
      const updated = { ...mockFeed, name: 'Updated Feed' }
      mockGeneratedClient.PUT.mockReturnValue(success(updated))

      const body = { name: 'Updated Feed' }
      const result = await feedService.updateFeed(TEAM_ID, FEED_ID, body)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feeds/{feed_id}',
        { params: { path: { team_id: TEAM_ID, feed_id: FEED_ID } }, body }
      )
      expect(result.name).toBe('Updated Feed')
    })
  })

  describe('deleteFeed', () => {
    it('calls delete endpoint', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(success(undefined))

      await feedService.deleteFeed(TEAM_ID, FEED_ID)

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feeds/{feed_id}',
        { params: { path: { team_id: TEAM_ID, feed_id: FEED_ID } } }
      )
    })
  })

  describe('getFeedItems', () => {
    it('returns feed items response', async () => {
      const mockResponse: FeedItemListResponse = {
        items: [mockFeedItem],
        total_count: 1,
        page: 1,
        per_page: 20,
        total_pages: 1,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await feedService.getFeedItems(TEAM_ID)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feed-items',
        { params: { path: { team_id: TEAM_ID }, query: {} } }
      )
      expect(result.items).toHaveLength(1)
    })

    it('passes archived filter', async () => {
      const mockResponse: FeedItemListResponse = {
        items: [],
        total_count: 0,
        page: 1,
        per_page: 20,
        total_pages: 0,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      await feedService.getFeedItems(TEAM_ID, { archived: 'true' })

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feed-items',
        {
          params: { path: { team_id: TEAM_ID }, query: { archived: 'true' } },
        }
      )
    })
  })

  describe('getFeedItemsForFeed', () => {
    it('returns items for a specific feed', async () => {
      const mockResponse: FeedItemListResponse = {
        items: [mockFeedItem],
        total_count: 1,
        page: 1,
        per_page: 20,
        total_pages: 1,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await feedService.getFeedItemsForFeed(TEAM_ID, FEED_ID)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feeds/{feed_id}/items',
        {
          params: {
            path: { team_id: TEAM_ID, feed_id: FEED_ID },
            query: {},
          },
        }
      )
      expect(result.items[0].feed_id).toBe(FEED_ID)
    })
  })

  describe('getFeedItem', () => {
    it('returns a single feed item', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockFeedItem))

      const result = await feedService.getFeedItem(TEAM_ID, ITEM_ID)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feed-items/{item_id}',
        { params: { path: { team_id: TEAM_ID, item_id: ITEM_ID } } }
      )
      expect(result.title).toBe('Sprint Retrospective')
    })
  })

  describe('createFeedItem', () => {
    it('creates a feed item', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockFeedItem))

      const body = {
        title: 'Sprint Retrospective',
        content: '## Summary\nAll tasks completed.',
        ai_assistant_name: 'claude-sonnet-4-5',
      }
      const result = await feedService.createFeedItem(TEAM_ID, FEED_ID, body)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feeds/{feed_id}/items',
        { params: { path: { team_id: TEAM_ID, feed_id: FEED_ID } }, body }
      )
      expect(result.id).toBe(ITEM_ID)
    })
  })

  describe('archiveFeedItem', () => {
    it('calls archive endpoint', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(undefined))

      await feedService.archiveFeedItem(TEAM_ID, ITEM_ID)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feed-items/{item_id}/archive',
        { params: { path: { team_id: TEAM_ID, item_id: ITEM_ID } } }
      )
    })
  })

  describe('unarchiveFeedItem', () => {
    it('calls unarchive endpoint', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(undefined))

      await feedService.unarchiveFeedItem(TEAM_ID, ITEM_ID)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feed-items/{item_id}/unarchive',
        { params: { path: { team_id: TEAM_ID, item_id: ITEM_ID } } }
      )
    })
  })

  describe('deleteFeedItem', () => {
    it('calls delete endpoint', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(success(undefined))

      await feedService.deleteFeedItem(TEAM_ID, ITEM_ID)

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feed-items/{item_id}',
        { params: { path: { team_id: TEAM_ID, item_id: ITEM_ID } } }
      )
    })
  })

  describe('listReplies', () => {
    it('lists replies for a feed item', async () => {
      const mockResponse: FeedItemReplyListResponse = {
        replies: [mockReply],
        total_count: 1,
        page: 1,
        per_page: 20,
        total_pages: 1,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await feedService.listReplies(TEAM_ID, ITEM_ID, 1, 20)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feed-items/{item_id}/replies',
        {
          params: {
            path: { team_id: TEAM_ID, item_id: ITEM_ID },
            query: { page: 1, limit: 20 },
          },
        }
      )
      expect(result.replies).toHaveLength(1)
    })
  })

  describe('createReply', () => {
    it('creates a reply on a feed item', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockReply))

      const body = { content: 'Nice work' }
      const result = await feedService.createReply(TEAM_ID, ITEM_ID, body)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/feed-items/{item_id}/replies',
        { params: { path: { team_id: TEAM_ID, item_id: ITEM_ID } }, body }
      )
      expect(result.id).toBe('reply-1')
    })
  })

  describe('error handling', () => {
    it('throws ApiError with backend detail on RFC 9457 error', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/NOT_FOUND',
            title: 'Not Found',
            status: 404,
            detail: 'feed not found',
            code: 'NOT_FOUND',
            request_id: 'req-1',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 404, statusText: 'Not Found' },
        })
      )

      await expect(feedService.getFeed(TEAM_ID, FEED_ID)).rejects.toThrow(
        'feed not found'
      )
    })

    it('propagates errors from createFeed', async () => {
      mockGeneratedClient.POST.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/BAD_REQUEST',
            title: 'Bad Request',
            status: 400,
            detail: 'Validation failed',
            code: 'BAD_REQUEST',
            request_id: 'req-2',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 400, statusText: 'Bad Request' },
        })
      )

      await expect(
        feedService.createFeed(TEAM_ID, { name: '' })
      ).rejects.toThrow('Validation failed')
    })
  })
})
