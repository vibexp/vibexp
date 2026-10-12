import { CheckCircle2, Clock, MessageSquare } from 'lucide-react'
import { Link } from 'react-router'

import { Skeleton } from '@/components/ui/skeleton'
import { formatRelativeTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import type { FeedItem } from '@/services/feedService'

import {
  buildStaleGroups,
  type StaleGroup,
  unansweredFeedItems,
} from './dashboardLogic'
import {
  DASHBOARD_GROUP,
  DASHBOARD_ROW,
  DashboardSection,
} from './DashboardSection'
import { RESOURCE_TYPE_META } from './resourceTypeMeta'
import { useStaleResources } from './useDashboardResources'

/** Stale resources named per type before "and N more". */
const STALE_NAMES_PER_TYPE = 3
/** Unanswered feed posts listed before the rest are summarised. */
const UNANSWERED_SHOWN = 4

const LINK_CLASS =
  'hover:text-primary font-medium underline-offset-2 hover:underline'

function StaleRow({ group }: Readonly<{ group: StaleGroup }>) {
  const meta = RESOURCE_TYPE_META[group.type]
  const noun = group.total === 1 ? meta.singular : meta.plural
  return (
    <li className={DASHBOARD_ROW}>
      <Clock className="text-warning size-4 shrink-0" aria-hidden="true" />
      <div className="min-w-0 flex-1 text-sm">
        <Link to={group.listUrl} className={LINK_CLASS}>
          {group.total.toLocaleString()} stale {noun}
        </Link>
        <p className="text-muted-foreground truncate text-xs">
          {group.resources.map((resource, index) => (
            <span key={resource.id}>
              {index > 0 && ', '}
              {resource.url ? (
                <Link
                  to={resource.url}
                  className="hover:text-foreground hover:underline"
                >
                  {resource.name}
                </Link>
              ) : (
                resource.name
              )}
            </span>
          ))}
          {group.more > 0 && ` and ${group.more.toLocaleString()} more`}
        </p>
      </div>
      <Link
        to={group.listUrl}
        className="text-muted-foreground hover:text-foreground shrink-0 text-xs hover:underline"
      >
        Review
      </Link>
    </li>
  )
}

function UnansweredRow({ item }: Readonly<{ item: FeedItem }>) {
  return (
    <li className={DASHBOARD_ROW}>
      <MessageSquare
        className="text-muted-foreground size-4 shrink-0"
        aria-hidden="true"
      />
      <div className="min-w-0 flex-1 text-sm">
        <Link
          to={`/feeds/items/${encodeURIComponent(item.id)}`}
          className={cn(LINK_CLASS, 'block truncate')}
        >
          {item.title}
        </Link>
        <p className="text-muted-foreground truncate text-xs">
          {item.ai_assistant_name} · no replies yet
        </p>
      </div>
      <span className="text-muted-foreground shrink-0 text-xs">
        {formatRelativeTime(item.posted_at)}
      </span>
    </li>
  )
}

/**
 * The first block of the dashboard: what a person should act on, each row
 * linking straight to where to act.
 *
 * Built only from what the API already says: resources the freshness rules
 * mark stale, and feed posts with zero replies. Pending invitations are not
 * repeated here — the layout's own banner already shows them on every page.
 */
export function NeedsAttention({
  teamId,
  projectId,
  feedItems,
  feedLoading,
  feedError,
}: Readonly<{
  teamId: string | undefined
  projectId: string | undefined
  /** The team's recent active feed items, fetched once by the page. */
  feedItems: readonly FeedItem[]
  feedLoading: boolean
  feedError: boolean
}>) {
  const stale = useStaleResources(teamId, projectId, STALE_NAMES_PER_TYPE)

  const staleGroups = buildStaleGroups(stale.data ?? [], STALE_NAMES_PER_TYPE)
  const unanswered = unansweredFeedItems(feedItems)
  const shown = unanswered.slice(0, UNANSWERED_SHOWN)
  const hidden = unanswered.length - shown.length

  const loading = stale.loading || feedLoading
  const nothing = staleGroups.length === 0 && unanswered.length === 0
  // A source that could not be checked must not read as "all clear".
  const unchecked = [
    stale.error && 'stale resources',
    feedError && 'feed posts',
  ].filter(Boolean)

  let body
  if (loading) {
    body = (
      <ul className={DASHBOARD_GROUP} aria-busy="true">
        {[0, 1].map(i => (
          <li key={i} className={DASHBOARD_ROW}>
            <Skeleton className="size-4 rounded" />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-4 w-1/3" />
              <Skeleton className="h-3 w-1/2" />
            </div>
          </li>
        ))}
      </ul>
    )
  } else if (nothing) {
    body =
      unchecked.length === 0 ? (
        <p className="text-muted-foreground flex items-center gap-2 text-sm">
          <CheckCircle2 className="text-success size-4" aria-hidden="true" />
          Nothing needs your attention right now.
        </p>
      ) : null
  } else {
    body = (
      <ul className={DASHBOARD_GROUP}>
        {staleGroups.map(group => (
          <StaleRow key={group.type} group={group} />
        ))}
        {shown.map(item => (
          <UnansweredRow key={item.id} item={item} />
        ))}
        {hidden > 0 && (
          <li className={cn(DASHBOARD_ROW, 'text-sm')}>
            <Link
              to="/feeds"
              className="text-muted-foreground hover:text-foreground hover:underline"
            >
              {hidden.toLocaleString()} more feed{' '}
              {hidden === 1 ? 'post' : 'posts'} without a reply
            </Link>
          </li>
        )}
      </ul>
    )
  }

  return (
    <DashboardSection title="Needs your attention">
      {body}
      {!loading && unchecked.length > 0 && (
        <p className="text-muted-foreground text-sm">
          Could not check {unchecked.join(' or ')} just now.
        </p>
      )}
    </DashboardSection>
  )
}
