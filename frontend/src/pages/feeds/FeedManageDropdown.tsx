import { ChevronDown, Settings, Trash2 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { usePermissions } from '@/hooks/usePermissions'
import type { Feed } from '@/services/feedService'

interface FeedManageDropdownProps {
  feeds: Feed[]
  onDeleteFeed: (feed: Feed) => void
}

export function FeedManageDropdown({
  feeds,
  onDeleteFeed,
}: FeedManageDropdownProps) {
  const navigate = useNavigate()
  // Deleting a feed follows the same own-vs-any rule the backend applies to
  // feed items (FeedService.DeleteFeed), so it needs the same gate (#225).
  const { canDeleteFeedContent } = usePermissions()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          <Settings className="mr-2 size-4" />
          Manage feeds
          <ChevronDown className="ml-2 size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {feeds.map(feed => (
          <DropdownMenuItem
            key={feed.id}
            className="flex items-center justify-between gap-4"
            onSelect={e => {
              e.preventDefault()
            }}
          >
            <button
              type="button"
              className="flex-1 text-left text-sm"
              onClick={() => {
                void navigate(`/feeds/${encodeURIComponent(feed.id)}`)
              }}
            >
              {feed.name}
            </button>
            <div className="flex items-center gap-1">
              <button
                type="button"
                aria-label={`Edit ${feed.name}`}
                className="rounded p-1 hover:bg-muted"
                onClick={() => {
                  void navigate(`/feeds/${encodeURIComponent(feed.id)}/edit`)
                }}
              >
                <Settings className="size-3" />
              </button>
              {canDeleteFeedContent(feed.created_by_user_id) && (
                <button
                  type="button"
                  aria-label={`Delete ${feed.name}`}
                  className="rounded p-1 hover:bg-muted"
                  onClick={() => {
                    onDeleteFeed(feed)
                  }}
                >
                  <Trash2 className="size-3 text-destructive" />
                </button>
              )}
            </div>
          </DropdownMenuItem>
        ))}
        {feeds.length === 0 && (
          <DropdownMenuItem disabled>No feeds yet</DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
