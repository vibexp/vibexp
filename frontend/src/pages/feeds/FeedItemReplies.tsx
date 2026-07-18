import { MessageSquare } from 'lucide-react'
import { useEffect, useState } from 'react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { formatRelativeTime } from '@/lib/time'
import { FeedActorAvatar, resolveFeedActor } from '@/pages/feeds/feedActor'
import type { FeedItemReply } from '@/services/feedService'
import { feedService } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'
import { teamService } from '@/services/teamService'

interface FeedItemRepliesProps {
  teamId: string
  itemId: string
}

interface ReplyItemProps {
  reply: FeedItemReply
  member?: TeamMember
}

function ReplyItem({ reply, member }: Readonly<ReplyItemProps>) {
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

export function FeedItemReplies({
  teamId,
  itemId,
}: Readonly<FeedItemRepliesProps>) {
  const { handleError } = useErrorHandler()

  const [replies, setReplies] = useState<FeedItemReply[]>([])
  const [members, setMembers] = useState<Map<string, TeamMember>>(new Map())
  const [repliesLoading, setRepliesLoading] = useState(false)
  const [replyContent, setReplyContent] = useState('')
  const [submittingReply, setSubmittingReply] = useState(false)

  useEffect(() => {
    const loadData = async () => {
      try {
        setRepliesLoading(true)
        const [repliesResult, membersResult] = await Promise.allSettled([
          feedService.listReplies(teamId, itemId),
          teamService.getTeamMembers(teamId),
        ])
        if (repliesResult.status === 'fulfilled') {
          setReplies(repliesResult.value.replies)
        }
        if (membersResult.status === 'fulfilled') {
          const map = new Map(membersResult.value.map(m => [m.user_id, m]))
          setMembers(map)
        }
      } catch (err) {
        handleError(err, 'Failed to load replies')
      } finally {
        setRepliesLoading(false)
      }
    }
    void loadData()
  }, [teamId, itemId, handleError])

  const handleSubmitReply = async () => {
    if (!replyContent.trim()) return
    try {
      setSubmittingReply(true)
      const newReply = await feedService.createReply(teamId, itemId, {
        content: replyContent.trim(),
      })
      setReplies(prev => [newReply, ...prev])
      setReplyContent('')
    } catch (err) {
      handleError(err, 'Failed to post reply')
    } finally {
      setSubmittingReply(false)
    }
  }

  const repliesContent =
    replies.length === 0 ? (
      <p className="py-4 text-center text-sm text-muted-foreground">
        No replies yet
      </p>
    ) : (
      <div className="divide-y divide-border">
        {replies.map(reply => (
          <ReplyItem
            key={reply.id}
            reply={reply}
            member={members.get(reply.posted_by_user_id)}
          />
        ))}
      </div>
    )

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <MessageSquare className="size-4" />
          Replies
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Replies list */}
        {repliesLoading ? (
          <div className="flex justify-center py-4">
            <LoadingSpinner size="sm" />
          </div>
        ) : (
          repliesContent
        )}

        {/* Compose form — at the bottom of the thread */}
        <div className="flex items-end gap-2 pt-2">
          <Textarea
            rows={2}
            className="min-h-[60px] flex-1 resize-none"
            placeholder="Write a reply..."
            value={replyContent}
            onChange={e => {
              setReplyContent(e.target.value)
            }}
            onKeyDown={e => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void handleSubmitReply()
              }
            }}
            disabled={submittingReply}
          />
          <Button
            onClick={() => {
              void handleSubmitReply()
            }}
            disabled={submittingReply || !replyContent.trim()}
            size="sm"
          >
            {submittingReply ? 'Posting...' : 'Reply'}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
