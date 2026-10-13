import { useState } from 'react'
import { Link } from 'react-router'

import { marksLastWrite } from '@/components/lastWrite'
import { LastWriteIndicator } from '@/components/LastWriteIndicator'
import { RelativeTime } from '@/components/RelativeTime'
import { SegmentedControl } from '@/components/SegmentedControl'
import { cn } from '@/lib/utils'
import type { Activity } from '@/services/activityService'
import type { RecentComment } from '@/services/commentService'
import type { FeedItem } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'

import { mergeRecentChanges, type RecentChange } from './dashboardLogic'
import { DASHBOARD_ROW, DashboardSection } from './DashboardSection'
import {
  LIST_ROW_LINK_CLASS,
  ListBody,
  RecentActivityList,
  RecentCommentsList,
  RecentFeedList,
} from './RecentLists'
import { RESOURCE_TYPE_META } from './resourceTypeMeta'
import { useRecentResources } from './useDashboardResources'

/** Rows asked of each type, and rows shown once the four are merged. */
const PER_TYPE = 6
const SHOWN = 6

type Tab = 'resources' | 'feed' | 'comments' | 'activity'

const TABS = [
  { value: 'resources', label: 'Resources' },
  { value: 'feed', label: 'AI feed' },
  { value: 'comments', label: 'Comments' },
  { value: 'activity', label: 'Your activity' },
] as const

function ChangeRow({
  change,
  members,
}: Readonly<{ change: RecentChange; members: Map<string, TeamMember> }>) {
  const meta = RESOURCE_TYPE_META[change.type]
  const Icon = meta.icon
  // Only a creation names a person: the API records who created a resource,
  // not who last edited it.
  const creator =
    change.action === 'created' ? members.get(change.userId)?.name : undefined
  const verb = change.action === 'created' ? 'Created' : 'Updated'

  const row = (
    <>
      <div className="bg-muted text-foreground flex size-7 shrink-0 items-center justify-center rounded-md">
        <Icon className="size-3.5" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <p
          className={cn(
            'truncate text-sm font-medium',
            change.nameIsFallback && 'text-muted-foreground font-normal'
          )}
        >
          {change.name}
        </p>
        <p className="text-muted-foreground truncate text-xs">
          {verb} {meta.singular}
          {creator ? ` · ${creator}` : ''}
          {marksLastWrite(change.lastWriteSource) && ' · '}
          <LastWriteIndicator
            variant="inline"
            source={change.lastWriteSource}
            assistantName={change.lastWriteAssistantName}
          />
        </p>
      </div>
      <span className="text-muted-foreground shrink-0 text-xs">
        <RelativeTime value={change.updatedAt} />
      </span>
    </>
  )

  return (
    <li>
      {change.url ? (
        <Link to={change.url} className={LIST_ROW_LINK_CLASS}>
          {row}
        </Link>
      ) : (
        <div className={DASHBOARD_ROW}>{row}</div>
      )}
    </li>
  )
}

/**
 * The second block of the dashboard: what changed lately. One bordered group
 * with a view switch in place of the three separate cards that used to sit at
 * the bottom of the page — resources (creations and updates across every
 * type, merged and time-ordered), the AI feed, comments, and the reader's own
 * activity.
 */
export function WhatChanged({
  teamId,
  projectId,
  members,
  feed,
  comments,
  activities,
}: Readonly<{
  teamId: string | undefined
  projectId: string | undefined
  members: Map<string, TeamMember>
  feed: { items: readonly FeedItem[]; loading: boolean; error: string | null }
  comments: {
    items: RecentComment[]
    loading: boolean
    error: string | null
  }
  activities: {
    items: readonly Activity[]
    loading: boolean
    error: string | null
  }
}>) {
  const [tab, setTab] = useState<Tab>('resources')
  const recent = useRecentResources(teamId, projectId, PER_TYPE)
  const changes = mergeRecentChanges(recent.data ?? [], SHOWN)

  return (
    <DashboardSection
      title="What changed"
      aside={
        <SegmentedControl
          size="sm"
          aria-label="What changed view"
          options={TABS}
          value={tab}
          onChange={value => {
            setTab(value as Tab)
          }}
        />
      }
    >
      <div className="overflow-hidden rounded-lg border">
        {tab === 'resources' && (
          <ListBody
            loading={recent.loading}
            error={recent.error ? 'Failed to load recent changes' : null}
            isEmpty={changes.length === 0}
            emptyMessage="Resources your team creates or updates will appear here."
          >
            <ul className="divide-y">
              {changes.map(change => (
                <ChangeRow
                  key={`${change.type}:${change.id}`}
                  change={change}
                  members={members}
                />
              ))}
            </ul>
          </ListBody>
        )}
        {tab === 'feed' && (
          <RecentFeedList
            items={feed.items}
            loading={feed.loading}
            error={feed.error}
          />
        )}
        {tab === 'comments' && (
          <RecentCommentsList
            comments={comments.items}
            members={members}
            loading={comments.loading}
            error={comments.error}
          />
        )}
        {tab === 'activity' && (
          <RecentActivityList
            activities={activities.items}
            loading={activities.loading}
            error={activities.error}
          />
        )}
      </div>
    </DashboardSection>
  )
}
