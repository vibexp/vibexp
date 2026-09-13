import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { Badge } from '@/components/ui/badge'
import { formatRelativeTime } from '@/lib/time'
import { FeedActorAvatar, resolveFeedActor } from '@/pages/feeds/feedActor'
import type { FeedItemReply } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'

interface ReplyItemProps {
  reply: FeedItemReply
  member?: TeamMember
}

/** One feed item reply row — shared by the replies panel and its "all replies" popup. */
export function ReplyItem({ reply, member }: Readonly<ReplyItemProps>) {
  const actor = resolveFeedActor(reply, member)

  return (
    <div className="flex gap-3 py-3">
      <FeedActorAvatar actor={actor} size="sm" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5 text-sm">
          <span className="font-semibold text-foreground">
            {actor.displayName}
          </span>
          {actor.isAi && (
            <Badge variant="outline" className="text-xs ml-1">
              AI
            </Badge>
          )}
          <span aria-hidden="true" className="text-muted-foreground">
            ·
          </span>
          <span className="text-muted-foreground">
            {formatRelativeTime(reply.posted_at)}
          </span>
        </div>
        <MarkdownRenderer
          content={reply.content}
          syntaxTheme="auto"
          className="mt-0.5 text-sm"
        />
      </div>
    </div>
  )
}
