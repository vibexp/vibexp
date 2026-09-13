import { MessageSquare } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { FeedItemReply } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'

import { ReplyComposer } from './ReplyComposer'
import { ReplyItem } from './ReplyItem'

interface AllRepliesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  replies: FeedItemReply[]
  members: Map<string, TeamMember>
  totalCount: number
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
  onAdd: (content: string) => Promise<void>
}

/**
 * "All replies" popup: the full, paginated reply thread of a feed item. Posting
 * works exactly as in the panel — the two share one `useFeedReplies` instance,
 * so a reply added here shows up (and is counted) in the panel behind it.
 */
export function AllRepliesDialog({
  open,
  onOpenChange,
  replies,
  members,
  totalCount,
  hasMore,
  loadingMore,
  onLoadMore,
  onAdd,
}: Readonly<AllRepliesDialogProps>) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] gap-0 overflow-hidden p-0 sm:max-w-lg">
        <DialogHeader className="border-border border-b px-6 py-4">
          <DialogTitle className="flex items-center gap-2">
            <MessageSquare className="size-4" />
            Replies ({totalCount})
          </DialogTitle>
        </DialogHeader>

        <div className="max-h-[70vh] overflow-y-auto px-6 py-4">
          <ReplyComposer onSubmit={onAdd} className="pb-2" />

          <div className="divide-border divide-y">
            {replies.map(reply => (
              <ReplyItem
                key={reply.id}
                reply={reply}
                member={members.get(reply.posted_by_user_id)}
              />
            ))}
          </div>

          {hasMore && (
            <div className="flex justify-center pt-4">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={loadingMore}
                onClick={onLoadMore}
              >
                {loadingMore ? 'Loading…' : 'Load more replies'}
              </Button>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
