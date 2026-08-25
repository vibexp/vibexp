import type { Team } from '@/services/teamService'

/**
 * The embedding-provider copy endpoint authorizes `team.update` on BOTH teams
 * (#831), so the action is only offered when the destination grants it and at
 * least one other team the user belongs to does too.
 *
 * Never gate on `team.role` — the permission matrix lives on the server and is
 * published as the `permissions` array (#224); UI gating is convenience only,
 * since every write is authorized server-side regardless.
 */
export function canCopyEmbeddingProviderFrom(candidate: Team) {
  return candidate.permissions.includes('team.update')
}
