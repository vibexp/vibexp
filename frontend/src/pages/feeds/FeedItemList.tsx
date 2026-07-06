import { Rss } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { FeedItemCard } from '@/pages/feeds/FeedItemCard'
import type { FeedItem } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'

interface FeedItemListProps {
  items: FeedItem[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  tab: 'active' | 'archived'
  hasFilters: boolean
  feedName?: (feedId: string) => string | undefined
  projectName?: (projectId: string | null | undefined) => string | undefined
  member?: (userId: string | null | undefined) => TeamMember | undefined
  onArchive?: (item: FeedItem) => Promise<void>
  onUnarchive?: (item: FeedItem) => Promise<void>
  onDelete: (item: FeedItem) => void
  onPagePrev: () => void
  onPageNext: () => void
  showMcpHint?: boolean
}

export function FeedItemList({
  items,
  loading,
  error,
  totalPages,
  currentPage,
  tab,
  hasFilters,
  feedName,
  projectName,
  member,
  onArchive,
  onUnarchive,
  onDelete,
  onPagePrev,
  onPageNext,
  showMcpHint = false,
}: FeedItemListProps) {
  const navigate = useNavigate()

  if (loading) {
    return (
      <div className="space-y-3">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-24 w-full rounded-lg" />
        ))}
      </div>
    )
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Failed to load feed items</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }

  if (items.length === 0) {
    return (
      <EmptyState
        icon={Rss}
        title={
          hasFilters
            ? 'No feed items match your filters'
            : tab === 'archived'
              ? 'No archived feed items'
              : 'No feed items yet'
        }
        description={
          hasFilters
            ? 'Try different search or filter settings.'
            : tab === 'archived'
              ? 'Archived feed items will appear here.'
              : 'Feed items are posted by AI assistants via MCP. Set up your MCP integration to start receiving AI-generated content.'
        }
        actions={
          showMcpHint && !hasFilters && tab === 'active' ? (
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/mcp-servers/vibexp-mcp')
              }}
            >
              MCP setup
            </Button>
          ) : undefined
        }
      />
    )
  }

  return (
    <div className="space-y-3">
      {items.map(item => (
        <FeedItemCard
          key={item.id}
          item={item}
          feedName={feedName?.(item.feed_id)}
          projectName={projectName?.(item.project_id)}
          member={member?.(item.posted_by_user_id)}
          replyCount={item.reply_count}
          onArchive={tab === 'active' ? onArchive : undefined}
          onUnarchive={tab === 'archived' ? onUnarchive : undefined}
          onDelete={onDelete}
        />
      ))}
      {totalPages > 1 && (
        <div className="flex items-center justify-between gap-2 pt-2">
          <div className="text-muted-foreground text-sm">
            Page {currentPage} of {totalPages}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={currentPage <= 1}
              onClick={onPagePrev}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={currentPage >= totalPages}
              onClick={onPageNext}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
