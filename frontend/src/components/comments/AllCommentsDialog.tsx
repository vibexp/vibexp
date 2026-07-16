import { MessageSquare } from 'lucide-react'

import { EmptyState } from '@/components/EmptyState'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { Comment } from '@/services/commentService'
import type { TeamMember } from '@/services/teamService'

import { CommentComposer } from './CommentComposer'
import { CommentRow } from './CommentRow'

interface AllCommentsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  comments: Comment[]
  members: Map<string, TeamMember>
  totalCount: number
  hasMore: boolean
  loadingMore: boolean
  canComment: boolean
  currentUserId?: string
  canDeleteResource: (resourceOwnerId: string | undefined) => boolean
  onLoadMore: () => void
  onAdd: (content: string) => Promise<void>
  onEdit: (commentId: string, content: string) => Promise<void>
  onDelete: (comment: Comment) => void
}

/**
 * "All comments" popup: the full, paginated comment list. Add/edit/delete work
 * identically to the sidebar widget — the two share one `useComments` instance,
 * so a change here is reflected in the widget behind it. Bodies are not clamped.
 */
export function AllCommentsDialog({
  open,
  onOpenChange,
  comments,
  members,
  totalCount,
  hasMore,
  loadingMore,
  canComment,
  currentUserId,
  canDeleteResource,
  onLoadMore,
  onAdd,
  onEdit,
  onDelete,
}: AllCommentsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] gap-0 overflow-hidden p-0 sm:max-w-lg">
        <DialogHeader className="border-border border-b px-6 py-4">
          <DialogTitle className="flex items-center gap-2">
            <MessageSquare className="size-4" />
            Comments ({totalCount})
          </DialogTitle>
        </DialogHeader>

        <div className="max-h-[70vh] overflow-y-auto px-6 py-4">
          {canComment && (
            <div className="pb-2">
              <CommentComposer onSubmit={onAdd} submitLabel="Comment" />
            </div>
          )}

          {comments.length === 0 ? (
            <EmptyState
              icon={MessageSquare}
              title="No comments yet"
              description="Be the first to leave a comment on this resource."
              className="border-none p-8"
            />
          ) : (
            <div className="divide-border divide-y">
              {comments.map(comment => (
                <CommentRow
                  key={comment.id}
                  comment={comment}
                  member={members.get(comment.user_id)}
                  currentUserId={currentUserId}
                  canDeleteResource={canDeleteResource}
                  onEdit={onEdit}
                  onDelete={onDelete}
                />
              ))}
            </div>
          )}

          {hasMore && (
            <div className="flex justify-center pt-4">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={loadingMore}
                onClick={onLoadMore}
              >
                {loadingMore ? 'Loading…' : 'Load more comments'}
              </Button>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
