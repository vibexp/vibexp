import type { components } from '@vibexp/api-client'

/**
 * Human labels for the audit log's operations, grouped the way the operation
 * filter shows them (#1517). The one place the vocabulary is translated, so the
 * table, the cards, the drawer and the filter can never word an operation two
 * ways.
 *
 * `Record<AuditOperation, …>` makes a missing label a `tsc -b` failure, and
 * `operationLabels.test.ts` checks the map against the published client's enum
 * at runtime too, so an operation added to the API cannot reach the page
 * unlabelled.
 */

export type AuditOperation = components['schemas']['AdminAuditOperation']

export type AuditOperationGroupId =
  'resources' | 'auth' | 'team' | 'instance' | 'authorization' | 'audit'

export interface AuditOperationInfo {
  label: string
  group: AuditOperationGroupId
}

export const AUDIT_OPERATIONS: Record<AuditOperation, AuditOperationInfo> = {
  'prompt.create': { label: 'Prompt created', group: 'resources' },
  'prompt.update': { label: 'Prompt updated', group: 'resources' },
  'prompt.move': { label: 'Prompt moved', group: 'resources' },
  'prompt.delete': { label: 'Prompt deleted', group: 'resources' },
  'prompt.lock': { label: 'Prompt locked', group: 'resources' },
  'prompt.unlock': { label: 'Prompt unlocked', group: 'resources' },
  'blueprint.create': { label: 'Blueprint created', group: 'resources' },
  'blueprint.update': { label: 'Blueprint updated', group: 'resources' },
  'blueprint.move': { label: 'Blueprint moved', group: 'resources' },
  'blueprint.delete': { label: 'Blueprint deleted', group: 'resources' },
  'blueprint.lock': { label: 'Blueprint locked', group: 'resources' },
  'blueprint.unlock': { label: 'Blueprint unlocked', group: 'resources' },
  'artifact.create': { label: 'Artifact created', group: 'resources' },
  'artifact.update': { label: 'Artifact updated', group: 'resources' },
  'artifact.move': { label: 'Artifact moved', group: 'resources' },
  'artifact.delete': { label: 'Artifact deleted', group: 'resources' },
  'artifact.lock': { label: 'Artifact locked', group: 'resources' },
  'artifact.unlock': { label: 'Artifact unlocked', group: 'resources' },
  'memory.create': { label: 'Memory created', group: 'resources' },
  'memory.update': { label: 'Memory updated', group: 'resources' },
  'memory.move': { label: 'Memory moved', group: 'resources' },
  'memory.delete': { label: 'Memory deleted', group: 'resources' },
  'feed.create': { label: 'Feed created', group: 'resources' },
  'feed.update': { label: 'Feed updated', group: 'resources' },
  'feed.delete': { label: 'Feed deleted', group: 'resources' },
  'feed_item.create': { label: 'Feed item created', group: 'resources' },
  'feed_item.delete': { label: 'Feed item deleted', group: 'resources' },
  'feed_item.archive': { label: 'Feed item archived', group: 'resources' },
  'feed_item.unarchive': { label: 'Feed item unarchived', group: 'resources' },
  'project.create': { label: 'Project created', group: 'resources' },
  'project.update': { label: 'Project updated', group: 'resources' },
  'project.delete': { label: 'Project deleted', group: 'resources' },
  'project.migrated': {
    label: 'Project migrated to another team',
    group: 'resources',
  },
  'auth.login': { label: 'Signed in', group: 'auth' },
  'auth.login_failed': { label: 'Sign-in failed', group: 'auth' },
  'auth.logout': { label: 'Signed out', group: 'auth' },
  'auth.setup_session.started': {
    label: 'Setup session started',
    group: 'auth',
  },
  'auth.sessions.revoked': { label: 'Sessions revoked', group: 'auth' },
  'auth.api_key.created': { label: 'API key created', group: 'auth' },
  'auth.api_key.revoked': { label: 'API key revoked', group: 'auth' },
  'auth.api_access.revoked': { label: 'API access revoked', group: 'auth' },
  'auth.password.set': { label: 'Password set', group: 'auth' },
  'auth.password.changed': { label: 'Password changed', group: 'auth' },
  'auth.password.removed': { label: 'Password removed', group: 'auth' },
  'auth.password.reset_requested': {
    label: 'Password reset requested',
    group: 'auth',
  },
  'auth.password.reset_link_issued': {
    label: 'Password reset link issued',
    group: 'auth',
  },
  'auth.identity.linked': { label: 'Identity linked', group: 'auth' },
  'auth.identity.unlinked': { label: 'Identity unlinked', group: 'auth' },
  'auth.mcp_client.registered': {
    label: 'MCP client registered',
    group: 'auth',
  },
  'auth.mcp_consent.granted': { label: 'MCP access granted', group: 'auth' },
  'auth.mcp_consent.denied': { label: 'MCP access denied', group: 'auth' },
  'auth.mcp_grant.revoked': { label: 'MCP access revoked', group: 'auth' },
  'team.created': { label: 'Team created', group: 'team' },
  'team.updated': { label: 'Team updated', group: 'team' },
  'team.deleted': { label: 'Team deleted', group: 'team' },
  'team.invite.sent': { label: 'Invitation sent', group: 'team' },
  'team.invite.accepted': { label: 'Invitation accepted', group: 'team' },
  'team.invite.rejected': { label: 'Invitation declined', group: 'team' },
  'team.invite.revoked': { label: 'Invitation revoked', group: 'team' },
  'team.member.added': { label: 'Member added', group: 'team' },
  'team.member.role_changed': { label: 'Member role changed', group: 'team' },
  'team.member.removed': { label: 'Member removed', group: 'team' },
  'team.ownership.transferred': {
    label: 'Ownership transferred',
    group: 'team',
  },
  'instance.settings.updated': {
    label: 'Instance settings updated',
    group: 'instance',
  },
  'admin.user.created': { label: 'User created', group: 'instance' },
  'admin.user.updated': { label: 'User updated', group: 'instance' },
  'admin.user.suspended': { label: 'User suspended', group: 'instance' },
  'admin.user.unsuspended': { label: 'User reactivated', group: 'instance' },
  'admin.user.deleted': { label: 'User deleted', group: 'instance' },
  'authz.denied': { label: 'Permission denied', group: 'authorization' },
  'audit.export': { label: 'Audit log exported', group: 'audit' },
  'audit.settings.updated': { label: 'Audit settings updated', group: 'audit' },
}

/** The operation filter's groups, in display order. */
export const AUDIT_OPERATION_GROUPS: readonly {
  id: AuditOperationGroupId
  label: string
}[] = [
  { id: 'resources', label: 'Resources' },
  { id: 'auth', label: 'Auth' },
  { id: 'team', label: 'Team' },
  { id: 'instance', label: 'Instance' },
  { id: 'authorization', label: 'Authorization' },
  { id: 'audit', label: 'Audit' },
]

const KNOWN = AUDIT_OPERATIONS as Record<string, AuditOperationInfo | undefined>

/** Whether `value` is an operation this page knows. */
export function isAuditOperation(value: string): value is AuditOperation {
  return KNOWN[value] !== undefined
}

/**
 * The label for an operation. An operation newer than this build (the server
 * deployed ahead of the SPA) falls back to its dotted name rather than a blank.
 */
export function operationLabel(operation: string): string {
  return KNOWN[operation]?.label ?? operation
}

/** The operations of one group, in the API's order. */
export function operationsInGroup(
  group: AuditOperationGroupId
): AuditOperation[] {
  return (Object.keys(AUDIT_OPERATIONS) as AuditOperation[]).filter(
    operation => AUDIT_OPERATIONS[operation].group === group
  )
}
