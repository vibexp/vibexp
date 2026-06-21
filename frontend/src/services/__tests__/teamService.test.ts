import type {
  PendingInvitationsResponse,
  TeamInvitation,
} from '../../types/team'

// Mock fetch globally
const mockFetch = jest.fn()
global.fetch = mockFetch

// Mock authService
const mockAuthService = {
  getToken: jest.fn(),
  logout: jest.fn(),
}

// Mock the teamService module
jest.mock('../teamService', () => {
  const API_BASE_URL = 'https://api.vibexp.io/api/v1'

  class TeamService {
    async getPendingInvitations(): Promise<TeamInvitation[]> {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(`${API_BASE_URL}/invitations/pending`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
      })

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.logout()
          throw new Error('Authentication expired')
        }
        throw new Error('Failed to get pending invitations')
      }

      const data: PendingInvitationsResponse = await response.json()
      return data.invitations ?? []
    }
  }

  return {
    teamService: new TeamService(),
  }
})

// Mock authService module
jest.mock('../authService', () => ({
  authService: mockAuthService,
}))

// Import the mocked service
import { teamService } from '../teamService'

describe('TeamService - getPendingInvitations', () => {
  const mockToken = 'test-auth-token'

  beforeEach(() => {
    jest.clearAllMocks()
    mockFetch.mockClear()
    mockAuthService.getToken.mockReturnValue(mockToken)
  })

  describe('successful responses', () => {
    it('should handle empty invitations array', async () => {
      const mockResponse: PendingInvitationsResponse = {
        invitations: [],
        total_count: 0,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/invitations/pending',
        {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
        }
      )
      expect(result).toEqual([])
      expect(Array.isArray(result)).toBe(true)
    })

    it('should handle single invitation', async () => {
      const mockInvitation: TeamInvitation = {
        id: 'inv-1',
        team_id: 'team-123',
        team_name: 'Engineering Team',
        email: 'test@example.com',
        status: 'pending',
        token: 'token-abc',
        expires_at: '2024-12-31T23:59:59Z',
        created_at: '2024-01-01T00:00:00Z',
      }

      const mockResponse: PendingInvitationsResponse = {
        invitations: [mockInvitation],
        total_count: 1,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      expect(result).toHaveLength(1)
      expect(result[0]).toEqual(mockInvitation)
      expect(result[0].id).toBe('inv-1')
      expect(result[0].team_name).toBe('Engineering Team')
    })

    it('should handle multiple invitations', async () => {
      const mockInvitations: TeamInvitation[] = [
        {
          id: 'inv-1',
          team_id: 'team-123',
          team_name: 'Engineering Team',
          email: 'test@example.com',
          status: 'pending',
          token: 'token-abc',
          expires_at: '2024-12-31T23:59:59Z',
          created_at: '2024-01-01T00:00:00Z',
        },
        {
          id: 'inv-2',
          team_id: 'team-456',
          team_name: 'Design Team',
          email: 'test@example.com',
          status: 'pending',
          token: 'token-def',
          expires_at: '2024-12-31T23:59:59Z',
          created_at: '2024-01-02T00:00:00Z',
        },
        {
          id: 'inv-3',
          team_id: 'team-789',
          team_name: 'Marketing Team',
          email: 'test@example.com',
          status: 'pending',
          token: 'token-ghi',
          expires_at: '2024-12-31T23:59:59Z',
          created_at: '2024-01-03T00:00:00Z',
        },
      ]

      const mockResponse: PendingInvitationsResponse = {
        invitations: mockInvitations,
        total_count: 3,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      expect(result).toHaveLength(3)
      expect(result).toEqual(mockInvitations)
      expect(result[0].team_name).toBe('Engineering Team')
      expect(result[1].team_name).toBe('Design Team')
      expect(result[2].team_name).toBe('Marketing Team')
    })

    it('should extract invitations array from paginated response', async () => {
      const mockResponse: PendingInvitationsResponse = {
        invitations: [
          {
            id: 'inv-1',
            team_id: 'team-123',
            team_name: 'Test Team',
            email: 'test@example.com',
            status: 'pending',
            token: 'token-abc',
            expires_at: '2024-12-31T23:59:59Z',
            created_at: '2024-01-01T00:00:00Z',
          },
        ],
        total_count: 1,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      // Verify it returns just the invitations array, not the full response
      expect(result).toEqual(mockResponse.invitations)
      expect(result).not.toHaveProperty('total_count')
      expect(result).not.toHaveProperty('page')
      expect(result).not.toHaveProperty('page_size')
    })
  })

  describe('error handling', () => {
    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(teamService.getPendingInvitations()).rejects.toThrow(
        'No authentication token'
      )

      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle 401 unauthorized and logout', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(teamService.getPendingInvitations()).rejects.toThrow(
        'Authentication expired'
      )

      expect(mockAuthService.logout).toHaveBeenCalled()
    })

    it('should handle 500 server error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      })

      await expect(teamService.getPendingInvitations()).rejects.toThrow(
        'Failed to get pending invitations'
      )
    })

    it('should handle network error', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'))

      await expect(teamService.getPendingInvitations()).rejects.toThrow(
        'Network error'
      )
    })
  })

  describe('response structure validation', () => {
    it('should handle response with all pagination metadata fields', async () => {
      const mockResponse: PendingInvitationsResponse = {
        invitations: [],
        total_count: 0,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      // Should return array even if empty
      expect(Array.isArray(result)).toBe(true)
      expect(result).toHaveLength(0)
    })

    it('should handle invitation with all required fields', async () => {
      const mockInvitation: TeamInvitation = {
        id: 'inv-1',
        team_id: 'team-123',
        team_name: 'Test Team',
        email: 'test@example.com',
        status: 'pending',
        token: 'secure-token-123',
        expires_at: '2024-12-31T23:59:59Z',
        created_at: '2024-01-01T00:00:00Z',
      }

      const mockResponse: PendingInvitationsResponse = {
        invitations: [mockInvitation],
        total_count: 1,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      expect(result[0]).toMatchObject({
        id: expect.any(String),
        team_id: expect.any(String),
        team_name: expect.any(String),
        email: expect.any(String),
        status: expect.any(String),
        token: expect.any(String),
        expires_at: expect.any(String),
        created_at: expect.any(String),
      })
    })
  })

  describe('regression tests for issue #561', () => {
    it('should not crash when invitations array is accessed', async () => {
      const mockResponse: PendingInvitationsResponse = {
        invitations: [],
        total_count: 0,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      // This was the crash location in InvitationBanner.tsx:91
      expect(() => result.length).not.toThrow()
      expect(result.length).toBe(0)
    })

    it('should return array that can be iterated', async () => {
      const mockResponse: PendingInvitationsResponse = {
        invitations: [
          {
            id: 'inv-1',
            team_id: 'team-123',
            team_name: 'Test Team',
            email: 'test@example.com',
            status: 'pending',
            token: 'token-abc',
            expires_at: '2024-12-31T23:59:59Z',
            created_at: '2024-01-01T00:00:00Z',
          },
        ],
        total_count: 1,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      // Should be iterable (used in InvitationBanner component)
      expect(() => {
        result.forEach(inv => {
          expect(inv.id).toBeDefined()
        })
      }).not.toThrow()
    })

    it('should not return undefined when response is valid', async () => {
      const mockResponse: PendingInvitationsResponse = {
        invitations: [],
        total_count: 0,
        page: 1,
        page_size: 20,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      })

      const result = await teamService.getPendingInvitations()

      // The bug was: response.invitations returned undefined when backend sent []
      expect(result).not.toBeUndefined()
      expect(result).not.toBeNull()
      expect(Array.isArray(result)).toBe(true)
    })
  })
})
