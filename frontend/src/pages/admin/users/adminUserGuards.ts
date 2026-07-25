/** The part of the signed-in identity these guards need. */
export interface ActingAdmin {
  id: string
}

interface TargetUser {
  id: string
}

/**
 * Whether the acting admin may run a destructive action against this user.
 *
 * The server refuses two cases with a 409 (#454/#455): acting on **yourself**,
 * and acting on a **config-listed instance admin**. Only the first is knowable
 * here — neither `AdminUserListItem` nor `AdminUserDetail` exposes
 * `is_instance_admin`, so the client cannot identify a config admin at all. The
 * second case is therefore handled where it surfaces: the 409 is rendered as a
 * message rather than swallowed, so an admin who tries it is told why it failed.
 *
 * Gating here is convenience only — every write is authorized server-side
 * regardless. It exists so an admin is not offered an action that is certain to
 * fail, not as a security boundary.
 */
export function canActOn(
  target: TargetUser | null | undefined,
  actingAdmin: ActingAdmin | null | undefined
): boolean {
  if (!target || !actingAdmin) return false
  return target.id !== actingAdmin.id
}

/**
 * Why the action is unavailable, for a tooltip or helper line. `null` when it is
 * available.
 */
export function actionBlockedReason(
  target: TargetUser | null | undefined,
  actingAdmin: ActingAdmin | null | undefined
): string | null {
  if (!target || !actingAdmin) return 'Sign-in state is still resolving.'
  if (target.id === actingAdmin.id) {
    return 'You cannot suspend or delete your own account.'
  }
  return null
}
