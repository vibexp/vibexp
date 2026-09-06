import { ArrowRight, MessageSquare, Plus } from 'lucide-react'
import { useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Button } from '@/components/ui/button'
import {
  Panel,
  PanelAction,
  PanelHeader,
  PanelTitle,
  usePanelInset,
} from '@/components/ui/panel'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/useAuth'
import { useAlerts } from '@/hooks'
import { useComments } from '@/hooks/useComments'
import { usePermissions } from '@/hooks/usePermissions'
import { cn } from '@/lib/utils'
import type { Comment, CommentResourceType } from '@/services/commentService'
import { getErrorMessage } from '@/utils/errorHandling'

import { AllCommentsDialog } from './AllCommentsDialog'
import { CommentComposer } from './CommentComposer'
import { CommentRow } from './CommentRow'

// The widget shows the newest few; the popup paginates the rest.
const WIDGET_VISIBLE = 5

interface CommentsPanelProps {
  teamId: string
  resourceType: CommentResourceType
  resourceId: string
}

/**
 * Self-fetching sidebar comments widget for a resource detail page, mirroring
 * the AccessActivity / Attachments panels. Shows the 5 newest comments with
 * inline add, in-place edit, and delete-with-confirm; a footer opens the "all
 * comments" popup once there are more than 5. Both surfaces share one
 * `useComments` instance, so they stay in sync. Server authorizes every write;
 * the UI gating here is convenience only.
 *
 * Built on the `ui/panel` primitives, so it is a card wherever it is dropped on
 * a page and flat inside the reading page's details column (#890).
 */
export function CommentsPanel({
  teamId,
  resourceType,
  resourceId,
}: Readonly<CommentsPanelProps>) {
  const { user } = useAuth()
  const { can, canDeleteResource } = usePermissions()
  const { showError } = useAlerts()
  const state = useComments(teamId, resourceType, resourceId)
  const inset = usePanelInset()

  const [composing, setComposing] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<Comment | null>(null)
  const [deleting, setDeleting] = useState(false)

  const canComment = can('resource.create')
  const currentUserId = user?.id
  const visible = state.comments.slice(0, WIDGET_VISIBLE)

  const handleConfirmDelete = async () => {
    if (!pendingDelete) return
    setDeleting(true)
    try {
      await state.removeComment(pendingDelete.id)
      setPendingDelete(null)
    } catch (err) {
      showError(
        getErrorMessage(err, 'Failed to delete comment'),
        'Delete failed'
      )
    } finally {
      setDeleting(false)
    }
  }

  // Body: loading / error / empty / list
  const renderBody = () => {
    if (state.loading) {
      return (
        <div
          className={cn(inset, 'space-y-4 py-4')}
          data-testid="comments-loading"
        >
          {[0, 1, 2].map(i => (
            <div key={i} className="flex gap-3">
              <Skeleton className="size-8 shrink-0 rounded-full" />
              <div className="flex-1 space-y-2">
                <Skeleton className="h-3 w-1/3" />
                <Skeleton className="h-3 w-full" />
                <Skeleton className="h-3 w-2/3" />
              </div>
            </div>
          ))}
        </div>
      )
    }
    if (state.error) {
      return (
        <div
          className={cn(
            inset,
            'flex flex-col items-center gap-2 py-6 text-center'
          )}
        >
          <p className="text-muted-foreground text-sm">
            Couldn&apos;t load comments.
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={state.reload}
          >
            Retry
          </Button>
        </div>
      )
    }
    if (state.comments.length === 0) {
      return (
        <p
          className={cn(
            inset,
            'text-muted-foreground py-6 text-center text-sm'
          )}
        >
          No comments yet.
          {canComment && ' Be the first to leave one.'}
        </p>
      )
    }
    return (
      <div className={cn(inset, 'divide-border divide-y')}>
        {visible.map(comment => (
          <CommentRow
            key={comment.id}
            comment={comment}
            member={state.members.get(comment.user_id)}
            currentUserId={currentUserId}
            canDeleteResource={canDeleteResource}
            clamp
            onEdit={state.editComment}
            onDelete={setPendingDelete}
            onReadFull={() => {
              setDialogOpen(true)
            }}
          />
        ))}
      </div>
    )
  }

  return (
    <Panel data-testid="comments-panel">
      {/* Header: icon + title (left), Add comment chip (right) */}
      <PanelHeader>
        <div className="flex min-w-0 items-center gap-2.5">
          <MessageSquare className="text-muted-foreground size-[17px] shrink-0" />
          <PanelTitle>Comments</PanelTitle>
        </div>
        {canComment && !composing && (
          <PanelAction
            onClick={() => {
              setComposing(true)
            }}
            data-testid="comment-add-button"
          >
            <Plus className="size-3.5" />
            Add comment
          </PanelAction>
        )}
      </PanelHeader>

      {/* Inline compose at the top of the list */}
      {canComment && composing && (
        <div className={cn(inset, 'pb-4')}>
          <CommentComposer
            focusOnMount
            onSubmit={state.addComment}
            onSuccess={() => {
              setComposing(false)
            }}
            onCancel={() => {
              setComposing(false)
            }}
          />
        </div>
      )}

      <div className="bg-border h-px" />

      {renderBody()}

      {/* Footer: "See all comments (N)" — only when there are more than the widget shows */}
      {state.totalCount > WIDGET_VISIBLE && (
        <button
          type="button"
          onClick={() => {
            setDialogOpen(true)
          }}
          className={cn(
            inset,
            'text-foreground hover:bg-accent border-border flex w-full items-center gap-2 border-t py-3 text-sm font-medium transition-colors'
          )}
          data-testid="comments-see-all"
        >
          {'See all comments'}
          <span className="bg-secondary text-secondary-foreground rounded-full px-[7px] py-[3px] font-mono text-xs leading-none">
            {state.totalCount}
          </span>
          <ArrowRight
            aria-hidden="true"
            className="text-muted-foreground ml-auto size-[13px] shrink-0"
          />
        </button>
      )}

      <AllCommentsDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        comments={state.comments}
        members={state.members}
        totalCount={state.totalCount}
        hasMore={state.hasMore}
        loadingMore={state.loadingMore}
        canComment={canComment}
        currentUserId={currentUserId}
        canDeleteResource={canDeleteResource}
        onLoadMore={state.loadMore}
        onAdd={state.addComment}
        onEdit={state.editComment}
        onDelete={setPendingDelete}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={open => {
          if (!open) setPendingDelete(null)
        }}
        title="Delete comment?"
        description="This will permanently delete the comment. This action cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleConfirmDelete}
      />
    </Panel>
  )
}
