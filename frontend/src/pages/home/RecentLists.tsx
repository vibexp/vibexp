import {
  BookOpen,
  Brain,
  ChevronRight,
  FileText,
  type LucideIcon,
  Sparkles,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'

import { RelativeTime } from '@/components/RelativeTime'
import { Skeleton } from '@/components/ui/skeleton'
import { buildResourceUrl } from '@/lib/resourceUrl'
import { FeedActorAvatar, resolveFeedActor } from '@/pages/feeds/feedActor'
import { getActivityIcon } from '@/pages/home/activityHelpers'
import type { Activity as ActivityType } from '@/services/activityService'
import type { RecentComment } from '@/services/commentService'
import type { FeedItem } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'

export function getActivitySubtitle(activity: ActivityType): string {
  const parts: string[] = []
  if (activity.entity_type) {
    const name = activity.entity_name ?? activity.entity_id
    if (name) {
      parts.push(`${activity.entity_type}: ${name}`)
    } else {
      parts.push(activity.entity_type)
    }
  }
  return parts.join(' · ') || activity.activity_type
}

interface ListBodyProps {
  loading: boolean
  error: string | null
  isEmpty: boolean
  emptyMessage: string
  /** Where "See all" goes; omit for a list with no fuller page (comments). */
  viewAllTo?: string
  children: ReactNode
}

/** Row classes shared by every list in the "What changed" block. */
export const LIST_ROW_CLASS = 'flex items-center gap-3 px-4 py-2.5'
export const LIST_ROW_LINK_CLASS = `${LIST_ROW_CLASS} hover:bg-muted/40 transition-colors`

/**
 * Shared body for the lists inside the "What changed" block: loading
 * skeletons sized like the rows they stand in for, an error or empty line,
 * and an optional "See all" link. It draws no card of its own — the block
 * around it is the one bordered group, and the lists differ only in their
 * row markup, supplied as children.
 */
export function ListBody({
  loading,
  error,
  isEmpty,
  emptyMessage,
  viewAllTo,
  children,
}: Readonly<ListBodyProps>) {
  if (loading) {
    return (
      <ul className="divide-y" aria-busy="true">
        {Array.from({ length: 5 }).map((_, i) => (
          <li key={i} className={LIST_ROW_CLASS}>
            <Skeleton className="size-7 rounded-md" />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-4 w-2/3" />
              <Skeleton className="h-3 w-1/3" />
            </div>
            <Skeleton className="h-3 w-12" />
          </li>
        ))}
      </ul>
    )
  }
  if (error || isEmpty) {
    return (
      <p className="text-muted-foreground px-4 py-6 text-sm">
        {error ?? emptyMessage}
      </p>
    )
  }
  return (
    <>
      {children}
      {viewAllTo && (
        <div className="border-t px-4 py-2">
          <Link
            to={viewAllTo}
            className="text-muted-foreground hover:text-foreground inline-flex items-center text-xs hover:underline"
          >
            See all
            <ChevronRight className="ml-0.5 size-3.5" aria-hidden="true" />
          </Link>
        </div>
      )}
    </>
  )
}

export function RecentFeedList({
  items,
  loading,
  error,
}: Readonly<{
  items: readonly FeedItem[]
  loading: boolean
  error: string | null
}>) {
  return (
    <ListBody
      loading={loading}
      error={error}
      isEmpty={items.length === 0}
      emptyMessage="Recent AI feed items for your team will appear here."
      viewAllTo="/feeds"
    >
      <ul className="divide-y">
        {items.slice(0, 6).map(item => (
          <li key={item.id}>
            <Link
              to={`/feeds/items/${encodeURIComponent(item.id)}`}
              className={LIST_ROW_LINK_CLASS}
            >
              <div className="bg-muted text-foreground flex size-7 shrink-0 items-center justify-center rounded-md">
                <Sparkles className="size-3.5" />
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{item.title}</p>
                <p className="text-muted-foreground truncate text-xs">
                  {item.excerpt
                    ? `${item.ai_assistant_name} · ${item.excerpt}`
                    : item.ai_assistant_name}
                </p>
              </div>
              <span className="text-muted-foreground shrink-0 text-xs">
                <RelativeTime value={item.posted_at} />
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </ListBody>
  )
}

// Per-type glyph for a commented resource, consistent with the app's iconography
// (Sparkles=prompt / BookOpen=blueprint as in the homepage Quick actions).
const RESOURCE_ICONS: Record<string, LucideIcon> = {
  artifact: FileText,
  blueprint: BookOpen,
  prompt: Sparkles,
  memory: Brain,
}

export function RecentCommentsList({
  comments,
  members,
  loading,
  error,
}: Readonly<{
  comments: RecentComment[]
  members: Map<string, TeamMember>
  loading: boolean
  error: string | null
}>) {
  return (
    <ListBody
      loading={loading}
      error={error}
      isEmpty={comments.length === 0}
      emptyMessage="No comments yet in this team."
    >
      <ul className="divide-y">
        {comments.slice(0, 6).map(comment => {
          const actor = resolveFeedActor(
            { posted_by_user_id: comment.user_id },
            members.get(comment.user_id)
          )
          const edited =
            new Date(comment.updated_at).getTime() >
            new Date(comment.created_at).getTime()
          const action = edited ? 'edited a comment on' : 'commented on'
          const Icon = RESOURCE_ICONS[comment.resource_type] ?? FileText
          const url = buildResourceUrl({
            type: comment.resource_type,
            id: comment.resource_id,
            slug: comment.slug,
            projectId: comment.project_id,
          })
          // RecentComment has no id; a user can comment at most once per resource
          // at a given activity time, so this composite key is stable + unique.
          const key = `${comment.resource_type}:${comment.resource_id}:${comment.user_id}:${comment.updated_at}`

          const row = (
            <>
              <FeedActorAvatar actor={actor} size="sm" />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm">
                  <span className="font-medium">{actor.displayName}</span>{' '}
                  <span className="text-muted-foreground">{action}</span>
                </p>
                <p className="text-muted-foreground flex items-center gap-1 truncate text-xs">
                  <Icon className="size-3 shrink-0" aria-hidden="true" />
                  <span className="truncate">{comment.resource_title}</span>
                </p>
              </div>
              <span className="text-muted-foreground shrink-0 text-xs">
                <RelativeTime value={comment.updated_at} />
              </span>
            </>
          )

          return (
            <li key={key}>
              {url ? (
                <Link to={url} className={LIST_ROW_LINK_CLASS}>
                  {row}
                </Link>
              ) : (
                <div className={LIST_ROW_CLASS}>{row}</div>
              )}
            </li>
          )
        })}
      </ul>
    </ListBody>
  )
}

export function RecentActivityList({
  activities,
  loading,
  error,
}: Readonly<{
  activities: readonly ActivityType[]
  loading: boolean
  error: string | null
}>) {
  return (
    <ListBody
      loading={loading}
      error={error}
      isEmpty={activities.length === 0}
      emptyMessage="Your recent platform usage will appear here."
      viewAllTo="/settings/activities"
    >
      <ul className="divide-y">
        {activities.slice(0, 6).map(activity => {
          const Icon = getActivityIcon(
            activity.activity_type,
            activity.entity_type
          )
          return (
            <li key={activity.id} className={LIST_ROW_CLASS}>
              <div className="bg-muted text-foreground flex size-7 shrink-0 items-center justify-center rounded-md">
                <Icon className="size-3.5" />
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">
                  {activity.description}
                </p>
                <p className="text-muted-foreground truncate text-xs">
                  {getActivitySubtitle(activity)}
                </p>
              </div>
              <span className="text-muted-foreground shrink-0 text-xs">
                <RelativeTime value={activity.created_at} />
              </span>
            </li>
          )
        })}
      </ul>
    </ListBody>
  )
}
