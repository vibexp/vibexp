import { ArrowRight, MessageSquare } from 'lucide-react'
import { useState } from 'react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { Panel, PanelHeader, PanelTitle } from '@/components/ui/panel'
import { REPLIES_PAGE_SIZE, useFeedReplies } from '@/hooks/useFeedReplies'

import { AllRepliesDialog } from './AllRepliesDialog'
import { ReplyComposer } from './ReplyComposer'
import { ReplyItem } from './ReplyItem'

interface FeedItemRepliesProps {
  teamId: string
  itemId: string
}

export function FeedItemReplies({
  teamId,
  itemId,
}: Readonly<FeedItemRepliesProps>) {
  // One hook instance shared by the panel and the "all replies" popup, so the
  // two never disagree on the list or the count (#954).
  const state = useFeedReplies(teamId, itemId)
  const [dialogOpen, setDialogOpen] = useState(false)

  // The panel shows the first page; the popup appends the rest. Both the chip
  // and the footer read `totalCount` — `replies.length` would read "10" beside
  // a thread of 57.
  const visible = state.replies.slice(0, REPLIES_PAGE_SIZE)

  const repliesContent =
    visible.length === 0 ? (
      <p className="py-4 text-center text-sm text-muted-foreground">
        No replies yet
      </p>
    ) : (
      <div className="divide-y divide-border">
        {visible.map(reply => (
          <ReplyItem
            key={reply.id}
            reply={reply}
            member={state.members.get(reply.posted_by_user_id)}
          />
        ))}
      </div>
    )

  // Built on `ui/panel`, not `Card` (#890/#919): inside the reading page's
  // details column `PanelPresentationProvider value="flat"` strips the border,
  // shadow and inset. Only that surface renders this today — the rows below do
  // not apply `usePanelInset()`, so moving it onto a `card` surface would need
  // the 20px gutter adding first. The heading stays: the section's `aria-label`
  // is invisible and the rail tooltip only exists while the column is
  // collapsed, so `CommentsPanel` (the same thing in the same column) labels
  // itself too.
  return (
    <Panel className="space-y-4" data-testid="feed-item-replies-panel">
      <PanelHeader>
        <div className="flex min-w-0 items-center gap-2.5">
          <MessageSquare className="text-muted-foreground size-[17px] shrink-0" />
          <PanelTitle>Replies</PanelTitle>
        </div>
        {state.totalCount > 0 && (
          <span
            className="bg-secondary text-secondary-foreground rounded-full px-[7px] py-[3px] font-mono text-xs leading-none"
            data-testid="feed-item-replies-count"
          >
            {state.totalCount}
          </span>
        )}
      </PanelHeader>

      {/* Replies list */}
      {state.loading ? (
        <div className="flex justify-center py-4">
          <LoadingSpinner size="sm" />
        </div>
      ) : (
        repliesContent
      )}

      {/* Footer: "See all replies (N)" — only when the thread is longer than the panel shows */}
      {!state.loading && state.totalCount > visible.length && (
        <button
          type="button"
          onClick={() => {
            setDialogOpen(true)
          }}
          className="text-foreground hover:bg-accent border-border flex w-full items-center gap-2 border-t py-3 text-sm font-medium transition-colors"
          data-testid="feed-item-replies-see-all"
        >
          {'See all replies'}
          <span className="bg-secondary text-secondary-foreground rounded-full px-[7px] py-[3px] font-mono text-xs leading-none">
            {state.totalCount}
          </span>
          <ArrowRight
            aria-hidden="true"
            className="text-muted-foreground ml-auto size-[13px] shrink-0"
          />
        </button>
      )}

      {/* Compose form — at the bottom of the thread */}
      <ReplyComposer onSubmit={state.addReply} className="pt-2" />

      <AllRepliesDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        replies={state.replies}
        members={state.members}
        totalCount={state.totalCount}
        hasMore={state.hasMore}
        loadingMore={state.loadingMore}
        onLoadMore={state.loadMore}
        onAdd={state.addReply}
      />
    </Panel>
  )
}
