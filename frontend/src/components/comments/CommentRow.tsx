import { MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { useState } from 'react'

import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { RelativeTime } from '@/components/RelativeTime'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatDateTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import { FeedActorAvatar, resolveFeedActor } from '@/pages/feeds/feedActor'
import type { Comment } from '@/services/commentService'
import type { TeamMember } from '@/services/teamService'

import { CommentComposer } from './CommentComposer'

interface CommentRowProps {
  comment: Comment
  member?: TeamMember
  /** Signed-in user id, for the author-only edit gate. */
  currentUserId?: string
  /** Own-vs-any delete gate from usePermissions. */
  canDeleteResource: (resourceOwnerId: string | undefined) => boolean
  /** Clamp the body to 4 lines and reveal a "Read full comment" link (widget). */
  clamp?: boolean
  onEdit: (commentId: string, content: string) => Promise<void>
  onDelete: (comment: Comment) => void
  onReadFull?: () => void
}

/** Heuristic (cf. FeedItemCard): a body worth a "Read full comment" affordance. */
function isLongBody(content: string): boolean {
  return content.length > 280 || (content.match(/\n/g)?.length ?? 0) >= 4
}

export function CommentRow({
  comment,
  member,
  currentUserId,
  canDeleteResource,
  clamp = false,
  onEdit,
  onDelete,
  onReadFull,
}: CommentRowProps) {
  const [editing, setEditing] = useState(false)
  const actor = resolveFeedActor({ posted_by_user_id: comment.user_id }, member)
  const isEdited =
    new Date(comment.updated_at).getTime() >
    new Date(comment.created_at).getTime()
  const canEdit = !!currentUserId && comment.user_id === currentUserId
  const canDelete = canDeleteResource(comment.user_id)
  const showMenu = (canEdit || canDelete) && !editing
  const bodyClamped = clamp && isLongBody(comment.content)

  return (
    <div className="flex gap-3 py-3" data-testid="comment-row">
      <FeedActorAvatar actor={actor} size="sm" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5 text-sm">
          <span className="text-foreground truncate font-semibold">
            {actor.displayName}
          </span>
          <span aria-hidden="true" className="text-muted-foreground">
            ·
          </span>
          <RelativeTime
            value={comment.created_at}
            className="text-muted-foreground"
          />
          {isEdited && (
            <TooltipProvider>
              <Tooltip delayDuration={0}>
                <TooltipTrigger asChild>
                  <span className="text-muted-foreground cursor-default text-xs">
                    (edited)
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  Edited {formatDateTime(comment.updated_at)}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
          {showMenu && (
            <div className="ml-auto">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-7"
                    aria-label="Comment actions"
                  >
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {canEdit && (
                    <DropdownMenuItem
                      onSelect={() => {
                        setEditing(true)
                      }}
                    >
                      <Pencil className="mr-2 size-3.5" />
                      Edit
                    </DropdownMenuItem>
                  )}
                  {canDelete && (
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onSelect={() => {
                        onDelete(comment)
                      }}
                    >
                      <Trash2 className="mr-2 size-3.5" />
                      Delete
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          )}
        </div>

        {editing ? (
          <div className="mt-2">
            <CommentComposer
              initialValue={comment.content}
              submitLabel="Save"
              focusOnMount
              onSubmit={content => onEdit(comment.id, content)}
              onSuccess={() => {
                setEditing(false)
              }}
              onCancel={() => {
                setEditing(false)
              }}
            />
          </div>
        ) : (
          <>
            <div className={cn('mt-0.5', bodyClamped && 'line-clamp-4')}>
              <MarkdownRenderer
                content={comment.content}
                syntaxTheme="auto"
                className="text-sm"
              />
            </div>
            {bodyClamped && onReadFull && (
              <button
                type="button"
                className="text-primary mt-1 text-xs font-medium hover:underline"
                onClick={onReadFull}
              >
                Read full comment
              </button>
            )}
          </>
        )}
      </div>
    </div>
  )
}
