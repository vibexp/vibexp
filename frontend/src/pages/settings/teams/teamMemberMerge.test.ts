import type { TeamInvitation, TeamMember } from '@/services/teamService'

import {
  mergeMembersAndInvitations,
  pendingInvitationToMember,
} from './teamMemberMerge'

const makeMember = (overrides: Partial<TeamMember> = {}): TeamMember => ({
  user_id: 'user-1',
  email: 'alice@example.com',
  name: 'Alice',
  role: 'member',
  joined_at: '2024-01-01T00:00:00Z',
  invitation_status: 'accepted',
  ...overrides,
})

const makeInvitation = (
  overrides: Partial<TeamInvitation> = {}
): TeamInvitation => ({
  id: 'inv-1',
  token: 'token-1',
  team_id: 'team-1',
  team_name: 'Engineering',
  invitee_email: 'invitee@example.com',
  role: 'member',
  status: 'pending',
  created_at: '2024-02-01T00:00:00Z',
  expires_at: '2024-12-31T00:00:00Z',
  ...overrides,
})

describe('pendingInvitationToMember', () => {
  it('produces a synthetic user_id prefixed with inv: for table row stability', () => {
    const member = pendingInvitationToMember(makeInvitation({ id: 'abc-123' }))
    expect(member.user_id).toBe('inv:abc-123')
  })

  it('uses invitee_email for both email and a derived display name', () => {
    const member = pendingInvitationToMember(
      makeInvitation({ invitee_email: 'jane.doe@example.com' })
    )
    expect(member.email).toBe('jane.doe@example.com')
    expect(member.name).toBe('jane.doe')
  })

  it('uses the full email as the name when no @ is present', () => {
    const member = pendingInvitationToMember(
      makeInvitation({ invitee_email: 'noatsign' })
    )
    expect(member.name).toBe('noatsign')
  })

  it('marks the row with invitation_status pending', () => {
    const member = pendingInvitationToMember(makeInvitation())
    expect(member.invitation_status).toBe('pending')
  })

  it('carries the invitation token so the row can offer a copyable link (#249)', () => {
    const member = pendingInvitationToMember(
      makeInvitation({ token: 'tok-abc-123' })
    )
    expect(member.invitationToken).toBe('tok-abc-123')
  })

  it('leaves invitationToken undefined when the wire omits the token', () => {
    const member = pendingInvitationToMember(
      makeInvitation({ token: undefined })
    )
    expect(member.invitationToken).toBeUndefined()
  })

  it('uses the invitation role and falls back to member when absent', () => {
    expect(
      pendingInvitationToMember(makeInvitation({ role: 'admin' })).role
    ).toBe('admin')
    expect(
      pendingInvitationToMember(makeInvitation({ role: undefined })).role
    ).toBe('member')
  })

  it('uses the invitation created_at as joined_at for the row', () => {
    const member = pendingInvitationToMember(
      makeInvitation({ created_at: '2024-05-01T00:00:00Z' })
    )
    expect(member.joined_at).toBe('2024-05-01T00:00:00Z')
  })
})

describe('mergeMembersAndInvitations', () => {
  it('returns just the members when there are no invitations', () => {
    const members = [makeMember()]
    expect(mergeMembersAndInvitations(members, [])).toEqual(members)
  })

  it('appends pending invitations to the members list', () => {
    const result = mergeMembersAndInvitations(
      [makeMember()],
      [makeInvitation({ invitee_email: 'new@example.com' })]
    )
    expect(result).toHaveLength(2)
    expect(result[1]?.email).toBe('new@example.com')
    expect(result[1]?.invitation_status).toBe('pending')
  })

  it('filters out non-pending invitations', () => {
    const result = mergeMembersAndInvitations(
      [],
      [
        makeInvitation({ id: 'a', status: 'pending' }),
        makeInvitation({
          id: 'b',
          status: 'accepted',
          invitee_email: 'b@example.com',
        }),
        makeInvitation({
          id: 'c',
          status: 'rejected',
          invitee_email: 'c@example.com',
        }),
        makeInvitation({
          id: 'd',
          status: 'revoked',
          invitee_email: 'd@example.com',
        }),
      ]
    )
    expect(result).toHaveLength(1)
    expect(result[0]?.user_id).toBe('inv:a')
  })

  it('de-dupes by email when an invitation matches an accepted member (case-insensitive)', () => {
    const accepted = makeMember({ email: 'Alice@Example.com' })
    const stalePending = makeInvitation({
      invitee_email: 'alice@example.com',
    })
    const result = mergeMembersAndInvitations([accepted], [stalePending])
    expect(result).toHaveLength(1)
    expect(result[0]?.user_id).toBe('user-1')
  })

  it('drops pending invitations with empty emails', () => {
    const result = mergeMembersAndInvitations(
      [],
      [makeInvitation({ invitee_email: undefined })]
    )
    expect(result).toEqual([])
  })

  it('preserves accepted members order and appends pending after them', () => {
    const m1 = makeMember({ user_id: 'u1', email: 'm1@example.com' })
    const m2 = makeMember({ user_id: 'u2', email: 'm2@example.com' })
    const inv = makeInvitation({ invitee_email: 'p1@example.com' })
    const result = mergeMembersAndInvitations([m1, m2], [inv])
    expect(result.map(r => r.user_id)).toEqual(['u1', 'u2', 'inv:inv-1'])
  })

  it('handles multiple pending invitations together', () => {
    const result = mergeMembersAndInvitations(
      [],
      [
        makeInvitation({ id: 'a', invitee_email: 'a@example.com' }),
        makeInvitation({ id: 'b', invitee_email: 'b@example.com' }),
        makeInvitation({ id: 'c', invitee_email: 'c@example.com' }),
      ]
    )
    expect(result).toHaveLength(3)
    expect(result.every(r => r.invitation_status === 'pending')).toBe(true)
  })

  it('de-dupes by email even when leading/trailing whitespace differs', () => {
    const accepted = makeMember({ email: ' alice@example.com ' })
    const stalePending = makeInvitation({ invitee_email: 'alice@example.com' })
    const result = mergeMembersAndInvitations([accepted], [stalePending])
    expect(result).toHaveLength(1)
    expect(result[0]?.user_id).toBe('user-1')
  })

  it('drops pending rows whose email is whitespace-only', () => {
    const result = mergeMembersAndInvitations(
      [],
      [makeInvitation({ invitee_email: '   ' })]
    )
    expect(result).toEqual([])
  })
})
