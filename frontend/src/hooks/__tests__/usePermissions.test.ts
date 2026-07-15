import { renderHook } from '@testing-library/react'

import { usePermissions } from '@/hooks/usePermissions'
import type { Team } from '@/services/teamService'

const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

const mockUseAuth = jest.fn()
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

const makeTeam = (permissions: Team['permissions']): Team => ({
  id: 'team-1',
  owner_id: 'owner-1',
  name: 'Team',
  slug: 'team',
  description: '',
  is_personal: false,
  permissions,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
})

// The matrix is the server's (epic #220); these are the sets it publishes.
const MEMBER_PERMISSIONS: Team['permissions'] = [
  'resource.create',
  'resource.update.any',
  'resource.delete.own',
]
const ADMIN_PERMISSIONS: Team['permissions'] = [
  'team.update',
  'member.invite',
  'member.remove',
  'member.role.update',
  'project.create',
  'project.update',
  'project.delete',
  'resource.create',
  'resource.update.any',
  'resource.delete.own',
  'resource.delete.any',
  'feed.delete.any',
]

beforeEach(() => {
  mockUseAuth.mockReturnValue({ user: { id: 'user-1' } })
})

afterEach(() => {
  jest.clearAllMocks()
})

describe('usePermissions', () => {
  describe('can()', () => {
    it('grants exactly what the current team publishes', () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(ADMIN_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.can('project.create')).toBe(true)
      expect(result.current.can('member.invite')).toBe(true)
    })

    it('denies what the team does not publish', () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(ADMIN_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      // An admin holds neither of these — they stay owner-only (epic #220).
      expect(result.current.can('team.delete')).toBe(false)
      expect(result.current.can('team.transfer')).toBe(false)
    })

    it('denies a member the permissions the matrix withholds', () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(MEMBER_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.can('resource.create')).toBe(true)
      expect(result.current.can('project.create')).toBe(false)
      expect(result.current.can('resource.delete.any')).toBe(false)
    })

    it('fails closed with no current team', () => {
      mockUseTeam.mockReturnValue({ currentTeam: null })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.can('resource.create')).toBe(false)
    })

    it('reads the team it is handed rather than the current team', () => {
      // The team settings page can show a team that is not the current one, so
      // an explicit team must win — otherwise it would gate the wrong team.
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(MEMBER_PERMISSIONS) })

      const { result } = renderHook(() =>
        usePermissions(makeTeam(ADMIN_PERMISSIONS))
      )

      expect(result.current.can('project.create')).toBe(true)
    })

    it('permits nothing when explicitly handed null, rather than falling back to the current team', () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(ADMIN_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions(null))

      expect(result.current.can('project.create')).toBe(false)
    })
  })

  describe('canDeleteResource()', () => {
    it('lets a member delete their own resource', () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(MEMBER_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteResource('user-1')).toBe(true)
    })

    it("stops a member deleting someone else's resource", () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(MEMBER_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteResource('user-2')).toBe(false)
    })

    it("lets an admin delete someone else's resource via resource.delete.any", () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(ADMIN_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteResource('user-2')).toBe(true)
    })

    it('fails closed when the owner is unknown and the caller lacks delete.any', () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(MEMBER_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteResource(undefined)).toBe(false)
    })

    it('fails closed when the user is unknown and the caller lacks delete.any', () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(MEMBER_PERMISSIONS) })
      mockUseAuth.mockReturnValue({ user: null })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteResource('user-1')).toBe(false)
    })
  })

  describe('canDeleteFeedContent()', () => {
    it('lets an author delete their own post without feed.delete.any', () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(MEMBER_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteFeedContent('user-1')).toBe(true)
    })

    it("stops a member deleting someone else's post", () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(MEMBER_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteFeedContent('user-2')).toBe(false)
    })

    it("lets a moderator delete someone else's post via feed.delete.any", () => {
      mockUseTeam.mockReturnValue({ currentTeam: makeTeam(ADMIN_PERMISSIONS) })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteFeedContent('user-2')).toBe(true)
    })

    it('requires resource.delete.own for their own post, mirroring the backend', () => {
      // The backend passes (resource.delete.own, feed.delete.any) to
      // CanActOnResource — there is no feed.delete.own, so the "own" half is
      // the generic resource permission. Every real role happens to hold it, so
      // only a check like this keeps the two rules from silently diverging.
      mockUseTeam.mockReturnValue({
        currentTeam: makeTeam(['feed.delete.any']),
      })

      const { result } = renderHook(() => usePermissions())

      expect(result.current.canDeleteFeedContent('user-1')).toBe(false)
      expect(result.current.canDeleteFeedContent('user-2')).toBe(true)
    })
  })
})
