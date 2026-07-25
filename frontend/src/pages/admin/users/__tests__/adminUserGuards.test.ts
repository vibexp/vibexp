/**
 * canActOn / actionBlockedReason (#459).
 *
 * These decide whether the portal's destructive actions are offered at all, so
 * they get their own test rather than being covered incidentally by the page.
 */
import {
  actionBlockedReason,
  canActOn,
} from '@/pages/admin/users/adminUserGuards'

const admin = { id: 'admin-1' }

it('allows acting on another user', () => {
  expect(canActOn({ id: 'user-2' }, admin)).toBe(true)
  expect(actionBlockedReason({ id: 'user-2' }, admin)).toBeNull()
})

it('refuses acting on yourself', () => {
  // The server refuses this with a 409; offering the button would guarantee a
  // failed request, and an admin who locked themselves out could not undo it.
  expect(canActOn({ id: 'admin-1' }, admin)).toBe(false)
  expect(actionBlockedReason({ id: 'admin-1' }, admin)).toBe(
    'You cannot suspend or delete your own account.'
  )
})

it('fails closed while either identity is unresolved', () => {
  // A destructive action must never be enabled on the strength of missing data.
  expect(canActOn(null, admin)).toBe(false)
  expect(canActOn({ id: 'user-2' }, null)).toBe(false)
  expect(canActOn(undefined, undefined)).toBe(false)
  expect(actionBlockedReason(null, admin)).toBe(
    'Sign-in state is still resolving.'
  )
})

it('says nothing about config-listed instance admins, because it cannot know', () => {
  // Neither AdminUserListItem nor AdminUserDetail exposes is_instance_admin, so
  // the client cannot identify that case. It is handled where it surfaces: the
  // server's 409 is shown to the admin rather than swallowed. This test exists so
  // that a future schema change is a deliberate decision, not a surprise.
  const target = { id: 'user-2' } as { id: string; is_instance_admin?: boolean }
  target.is_instance_admin = true

  expect(canActOn(target, admin)).toBe(true)
})
