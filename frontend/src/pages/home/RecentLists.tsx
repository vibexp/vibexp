import {
  Activity as ActivityIcon,
  ChevronRight,
  type LucideIcon,
  Sparkles,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { formatRelativeTime } from '@/lib/time'
import { getActivityIcon } from '@/pages/home/activityHelpers'
import type { Activity as ActivityType } from '@/services/activityService'
import type { FeedItem } from '@/services/feedService'

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

interface ListCardProps {
  title: string
  count: number
  countLabel: string
  loading: boolean
  error: string | null
  isEmpty: boolean
  emptyIcon: LucideIcon
  emptyMessage: string
  onViewAll: () => void
  children: ReactNode
}

/** Shared shell for the two Activity lists: header + count, loading skeletons,
 * error/empty states, and a centered "See more" footer. The two lists differ
 * only in their row markup, supplied as children. */
function ListCard({
  title,
  count,
  countLabel,
  loading,
  error,
  isEmpty,
  emptyIcon: EmptyIcon,
  emptyMessage,
  onViewAll,
  children,
}: ListCardProps) {
  return (
    <Card className="flex flex-col">
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base">{title}</CardTitle>
        {!loading && !error && count > 0 && (
          <span className="text-muted-foreground text-xs">
            {count} {countLabel}
          </span>
        )}
      </CardHeader>
      <CardContent className="flex-1">
        {loading ? (
          <div className="space-y-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="flex items-center gap-3 py-2">
                <Skeleton className="size-9 rounded-md" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-4 w-2/3" />
                  <Skeleton className="h-3 w-1/3" />
                </div>
                <Skeleton className="h-3 w-12" />
              </div>
            ))}
          </div>
        ) : error ? (
          <div className="text-muted-foreground flex h-32 items-center justify-center text-sm">
            {error}
          </div>
        ) : isEmpty ? (
          <div className="text-muted-foreground flex h-32 flex-col items-center justify-center gap-2 text-center text-sm">
            <EmptyIcon className="size-5" />
            {emptyMessage}
          </div>
        ) : (
          children
        )}
      </CardContent>
      {!loading && !error && (
        <>
          <Separator />
          <CardFooter className="justify-center pt-4">
            <Button variant="ghost" size="sm" onClick={onViewAll}>
              See more
              <ChevronRight className="ml-1 size-4" />
            </Button>
          </CardFooter>
        </>
      )}
    </Card>
  )
}

export function RecentFeedList({
  items,
  loading,
  error,
  onViewAll,
}: {
  items: FeedItem[]
  loading: boolean
  error: string | null
  onViewAll: () => void
}) {
  return (
    <ListCard
      title="Recent AI feed"
      count={items.length}
      countLabel="updates"
      loading={loading}
      error={error}
      isEmpty={items.length === 0}
      emptyIcon={Sparkles}
      emptyMessage="Recent AI feed items for your team will appear here."
      onViewAll={onViewAll}
    >
      <ul className="divide-y">
        {items.slice(0, 8).map(item => (
          <li key={item.id}>
            <Link
              to={`/feed-items/${encodeURIComponent(item.id)}`}
              className="hover:bg-muted/40 -mx-2 flex items-center gap-3 rounded-md px-2 py-3 transition-colors"
            >
              <div className="bg-muted text-foreground flex size-9 shrink-0 items-center justify-center rounded-md">
                <Sparkles className="size-4" />
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
                {formatRelativeTime(item.posted_at)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </ListCard>
  )
}

export function RecentActivityList({
  activities,
  loading,
  error,
  onViewAll,
}: {
  activities: ActivityType[]
  loading: boolean
  error: string | null
  onViewAll: () => void
}) {
  return (
    <ListCard
      title="Recent activity"
      count={activities.length}
      countLabel="events"
      loading={loading}
      error={error}
      isEmpty={activities.length === 0}
      emptyIcon={ActivityIcon}
      emptyMessage="Your recent platform usage will appear here."
      onViewAll={onViewAll}
    >
      <ul className="divide-y">
        {activities.slice(0, 8).map(activity => {
          const Icon = getActivityIcon(
            activity.activity_type,
            activity.entity_type
          )
          return (
            <li
              key={activity.id}
              className="hover:bg-muted/40 -mx-2 flex items-center gap-3 rounded-md px-2 py-3 transition-colors"
            >
              <div className="bg-muted text-foreground flex size-9 shrink-0 items-center justify-center rounded-md">
                <Icon className="size-4" />
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
                {formatRelativeTime(activity.created_at)}
              </span>
            </li>
          )
        })}
      </ul>
    </ListCard>
  )
}
