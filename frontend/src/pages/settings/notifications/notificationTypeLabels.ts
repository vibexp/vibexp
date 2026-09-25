/**
 * Display labels for notification types, shared by the user's own settings page
 * and the admin's read-only view of them (#1137) so both name a type the same
 * way. An unknown type falls back to its raw key at the call site.
 */
export const NOTIFICATION_TYPE_LABELS: Record<string, string> = {
  'feed.item.created': 'New feed items',
  'feed.reply.created': 'Replies to your feed posts',
  'team.invitation': 'Team invitations',
}
