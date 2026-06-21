import type {
  Feed,
  FeedItem,
  FeedItemListResponse,
  FeedListResponse,
} from '../../types/feed'

// Mock apiClient
const mockGet = jest.fn()
const mockPost = jest.fn()
const mockPut = jest.fn()
const mockDelete = jest.fn()

jest.mock('../../lib/apiClient', () => ({
  apiClient: {
    get: (...args: unknown[]) => mockGet(...args),
    post: (...args: unknown[]) => mockPost(...args),
    put: (...args: unknown[]) => mockPut(...args),
    delete: (...args: unknown[]) => mockDelete(...args),
  },
}))

import { feedService } from '../feedService'

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
      mockGet.mockResolvedValueOnce(mockResponse)

      const result = await feedService.getFeeds(TEAM_ID)

      expect(mockGet).toHaveBeenCalledWith(`/${TEAM_ID}/feeds`)
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
      mockGet.mockResolvedValueOnce(mockResponse)

      await feedService.getFeeds(TEAM_ID, { page: 2, limit: 10 })

      expect(mockGet).toHaveBeenCalledWith(`/${TEAM_ID}/feeds?page=2&limit=10`)
    })
  })

  describe('getFeed', () => {
    it('returns a single feed', async () => {
      mockGet.mockResolvedValueOnce(mockFeed)

      const result = await feedService.getFeed(TEAM_ID, FEED_ID)

      expect(mockGet).toHaveBeenCalledWith(`/${TEAM_ID}/feeds/${FEED_ID}`)
      expect(result.id).toBe(FEED_ID)
    })
  })

  describe('createFeed', () => {
    it('creates a feed and returns it', async () => {
      mockPost.mockResolvedValueOnce(mockFeed)

      const result = await feedService.createFeed(TEAM_ID, {
        name: 'Product Updates',
        description: 'AI-generated updates',
      })

      expect(mockPost).toHaveBeenCalledWith(`/${TEAM_ID}/feeds`, {
        name: 'Product Updates',
        description: 'AI-generated updates',
      })
      expect(result.name).toBe('Product Updates')
    })
  })

  describe('updateFeed', () => {
    it('updates a feed and returns it', async () => {
      const updated = { ...mockFeed, name: 'Updated Feed' }
      mockPut.mockResolvedValueOnce(updated)

      const result = await feedService.updateFeed(TEAM_ID, FEED_ID, {
        name: 'Updated Feed',
      })

      expect(mockPut).toHaveBeenCalledWith(`/${TEAM_ID}/feeds/${FEED_ID}`, {
        name: 'Updated Feed',
      })
      expect(result.name).toBe('Updated Feed')
    })
  })

  describe('deleteFeed', () => {
    it('calls delete endpoint', async () => {
      mockDelete.mockResolvedValueOnce({})

      await feedService.deleteFeed(TEAM_ID, FEED_ID)

      expect(mockDelete).toHaveBeenCalledWith(`/${TEAM_ID}/feeds/${FEED_ID}`)
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
      mockGet.mockResolvedValueOnce(mockResponse)

      const result = await feedService.getFeedItems(TEAM_ID)

      expect(mockGet).toHaveBeenCalledWith(`/${TEAM_ID}/feed-items`)
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
      mockGet.mockResolvedValueOnce(mockResponse)

      await feedService.getFeedItems(TEAM_ID, { archived: 'true' })

      expect(mockGet).toHaveBeenCalledWith(
        `/${TEAM_ID}/feed-items?archived=true`
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
      mockGet.mockResolvedValueOnce(mockResponse)

      const result = await feedService.getFeedItemsForFeed(TEAM_ID, FEED_ID)

      expect(mockGet).toHaveBeenCalledWith(`/${TEAM_ID}/feeds/${FEED_ID}/items`)
      expect(result.items[0].feed_id).toBe(FEED_ID)
    })
  })

  describe('getFeedItem', () => {
    it('returns a single feed item', async () => {
      mockGet.mockResolvedValueOnce(mockFeedItem)

      const result = await feedService.getFeedItem(TEAM_ID, ITEM_ID)

      expect(mockGet).toHaveBeenCalledWith(`/${TEAM_ID}/feed-items/${ITEM_ID}`)
      expect(result.title).toBe('Sprint Retrospective')
    })
  })

  describe('createFeedItem', () => {
    it('creates a feed item', async () => {
      mockPost.mockResolvedValueOnce(mockFeedItem)

      const result = await feedService.createFeedItem(TEAM_ID, FEED_ID, {
        title: 'Sprint Retrospective',
        content: '## Summary\nAll tasks completed.',
        ai_assistant_name: 'claude-sonnet-4-5',
      })

      expect(mockPost).toHaveBeenCalledWith(
        `/${TEAM_ID}/feeds/${FEED_ID}/items`,
        {
          title: 'Sprint Retrospective',
          content: '## Summary\nAll tasks completed.',
          ai_assistant_name: 'claude-sonnet-4-5',
        }
      )
      expect(result.id).toBe(ITEM_ID)
    })
  })

  describe('archiveFeedItem', () => {
    it('calls archive endpoint', async () => {
      mockPost.mockResolvedValueOnce({})

      await feedService.archiveFeedItem(TEAM_ID, ITEM_ID)

      expect(mockPost).toHaveBeenCalledWith(
        `/${TEAM_ID}/feed-items/${ITEM_ID}/archive`
      )
    })
  })

  describe('unarchiveFeedItem', () => {
    it('calls unarchive endpoint', async () => {
      mockPost.mockResolvedValueOnce({})

      await feedService.unarchiveFeedItem(TEAM_ID, ITEM_ID)

      expect(mockPost).toHaveBeenCalledWith(
        `/${TEAM_ID}/feed-items/${ITEM_ID}/unarchive`
      )
    })
  })

  describe('deleteFeedItem', () => {
    it('calls delete endpoint', async () => {
      mockDelete.mockResolvedValueOnce({})

      await feedService.deleteFeedItem(TEAM_ID, ITEM_ID)

      expect(mockDelete).toHaveBeenCalledWith(
        `/${TEAM_ID}/feed-items/${ITEM_ID}`
      )
    })
  })

  describe('error handling', () => {
    it('propagates errors from getFeed', async () => {
      mockGet.mockRejectedValueOnce(new Error('Network error'))

      await expect(feedService.getFeed(TEAM_ID, FEED_ID)).rejects.toThrow(
        'Network error'
      )
    })

    it('propagates errors from createFeed', async () => {
      mockPost.mockRejectedValueOnce(new Error('Validation failed'))

      await expect(
        feedService.createFeed(TEAM_ID, { name: '' })
      ).rejects.toThrow('Validation failed')
    })
  })
})
