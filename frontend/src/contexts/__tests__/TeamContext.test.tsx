import { act, render, screen, waitFor } from '@testing-library/react'
import { renderHook } from '@testing-library/react'

import type { Team } from '../../services/teamService'
import { TeamProvider, useTeam } from '../TeamContext'

// Mock the teamService
jest.mock('../../services/teamService', () => ({
  teamService: {
    getTeams: jest.fn(),
  },
}))

// Mock the centralized storage utilities
jest.mock('../../utils/storage', () => ({
  storage: {
    get: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
  },
  sessionStore: {
    get: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
  },
}))

// Import the mocked modules after the mock
import { teamService } from '../../services/teamService'
import { storage } from '../../utils/storage'

// Type the mocked modules properly
const mockTeamService = teamService as jest.Mocked<typeof teamService>
const mockStorage = storage as jest.Mocked<typeof storage>

describe('TeamContext', () => {
  const mockTeams: Team[] = [
    {
      id: 'team-1',
      owner_id: 'owner-1',
      name: 'Team Alpha',
      slug: 'team-alpha',
      description: 'First team',
      role: 'owner',
      member_count: 5,
      is_personal: false,
      created_at: '2023-01-01T00:00:00Z',
      updated_at: '2023-01-01T00:00:00Z',
    },
    {
      id: 'team-2',
      owner_id: 'owner-1',
      name: 'Team Beta',
      slug: 'team-beta',
      description: 'Second team',
      role: 'member',
      member_count: 3,
      is_personal: false,
      created_at: '2023-01-01T00:00:00Z',
      updated_at: '2023-01-01T00:00:00Z',
    },
  ]

  beforeEach(() => {
    jest.clearAllMocks()
    // Clear mocks
    mockStorage.clear.mockImplementation(() => {})
    mockStorage.get.mockReturnValue(null)
    mockStorage.set.mockImplementation(() => {})
  })

  describe('Context Provider', () => {
    it('should fetch teams on mount and set first team as current', async () => {
      mockTeamService.getTeams.mockResolvedValue(mockTeams)

      const TestComponent = () => {
        const { currentTeam, teams, isLoading } = useTeam()
        return (
          <div>
            <div data-testid="current-team">
              {currentTeam ? currentTeam.name : 'null'}
            </div>
            <div data-testid="teams-count">{teams.length}</div>
            <div data-testid="loading">{String(isLoading)}</div>
          </div>
        )
      }

      render(
        <TeamProvider>
          <TestComponent />
        </TeamProvider>
      )

      // Wait for loading to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false')
      })

      expect(screen.getByTestId('current-team')).toHaveTextContent('Team Alpha')
      expect(screen.getByTestId('teams-count')).toHaveTextContent('2')
      expect(mockTeamService.getTeams).toHaveBeenCalledTimes(1)
    })

    it('should load team from storage if available', async () => {
      mockTeamService.getTeams.mockResolvedValue(mockTeams)
      // Simulate stored team ID
      mockStorage.get.mockReturnValueOnce('team-2')

      const TestComponent = () => {
        const { currentTeam, isLoading } = useTeam()
        return (
          <div>
            <div data-testid="current-team">
              {currentTeam ? currentTeam.name : 'null'}
            </div>
            <div data-testid="loading">{String(isLoading)}</div>
          </div>
        )
      }

      render(
        <TeamProvider>
          <TestComponent />
        </TeamProvider>
      )

      // Wait for loading to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false')
      })

      expect(screen.getByTestId('current-team')).toHaveTextContent('Team Beta')
    })

    it('should handle setCurrentTeam and update localStorage', async () => {
      mockTeamService.getTeams.mockResolvedValue(mockTeams)

      const TestComponent = () => {
        const { currentTeam, teams, setCurrentTeam, isLoading } = useTeam()
        return (
          <div>
            <div data-testid="current-team">
              {currentTeam ? currentTeam.name : 'null'}
            </div>
            <div data-testid="loading">{String(isLoading)}</div>
            <button
              onClick={() => {
                setCurrentTeam(teams[1])
              }}
              data-testid="switch-team"
            >
              Switch Team
            </button>
          </div>
        )
      }

      render(
        <TeamProvider>
          <TestComponent />
        </TeamProvider>
      )

      // Wait for loading to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false')
      })

      expect(screen.getByTestId('current-team')).toHaveTextContent('Team Alpha')

      // Switch team
      const switchBtn = screen.getByTestId('switch-team')
      act(() => {
        switchBtn.click()
      })

      expect(screen.getByTestId('current-team')).toHaveTextContent('Team Beta')
      expect(mockStorage.set).toHaveBeenCalledWith(
        expect.any(String), // STORAGE_KEYS.CURRENT_TEAM_ID
        'team-2'
      )
    })

    it('should throw error when useTeam is used outside TeamProvider', () => {
      const consoleErrorSpy = jest
        .spyOn(console, 'error')
        .mockImplementation(() => {})

      expect(() => {
        renderHook(() => useTeam())
      }).toThrow('useTeam must be used within TeamProvider')

      consoleErrorSpy.mockRestore()
    })

    it('should handle empty teams list', async () => {
      mockTeamService.getTeams.mockResolvedValue([])

      const TestComponent = () => {
        const { currentTeam, teams, isLoading } = useTeam()
        return (
          <div>
            <div data-testid="current-team">
              {currentTeam ? currentTeam.name : 'null'}
            </div>
            <div data-testid="teams-count">{teams.length}</div>
            <div data-testid="loading">{String(isLoading)}</div>
          </div>
        )
      }

      render(
        <TeamProvider>
          <TestComponent />
        </TeamProvider>
      )

      // Wait for loading to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false')
      })

      expect(screen.getByTestId('current-team')).toHaveTextContent('null')
      expect(screen.getByTestId('teams-count')).toHaveTextContent('0')
    })

    it('should handle team fetch error', async () => {
      mockTeamService.getTeams.mockRejectedValue(
        new Error('Failed to fetch teams')
      )

      const consoleErrorSpy = jest
        .spyOn(console, 'error')
        .mockImplementation(() => {})

      const TestComponent = () => {
        const { currentTeam, isLoading } = useTeam()
        return (
          <div>
            <div data-testid="current-team">
              {currentTeam ? currentTeam.name : 'null'}
            </div>
            <div data-testid="loading">{String(isLoading)}</div>
          </div>
        )
      }

      render(
        <TeamProvider>
          <TestComponent />
        </TeamProvider>
      )

      // Wait for loading to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false')
      })

      expect(screen.getByTestId('current-team')).toHaveTextContent('null')
      expect(consoleErrorSpy).toHaveBeenCalled()

      consoleErrorSpy.mockRestore()
    })
  })
})
