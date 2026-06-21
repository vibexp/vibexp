import { useEffect, useLayoutEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { formatRelativeTime } from '@/lib/time'
import {
  FeedActorAvatar,
  resolveFeedActor,
  USER_POST_ASSISTANT_NAME,
} from '@/pages/feeds/feedActor'
import { feedService } from '@/services/feedService'
import { teamService } from '@/services/teamService'
import type { FeedItemReply } from '@/types/feed'
import type { TeamMember } from '@/types/team'

interface InlineReplyThreadProps {
  itemId: string
  initialReplyCount?: number
  onReplyAdded?: () => void
}

const MAX_REPLY_LENGTH = 4000

export function InlineReplyThread({
  itemId,
  initialReplyCount,
  onReplyAdded,
}: InlineReplyThreadProps) {
  const { currentTeam } = useTeam()
  const { handleError } = useErrorHandler()

  const [replies, setReplies] = useState<FeedItemReply[]>([])
  const [members, setMembers] = useState<Map<string, TeamMember>>(new Map())
  const [loading, setLoading] = useState(true)
  const [draft, setDraft] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const draftRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    if (!currentTeam) return
    const ctrl = new AbortController()
    const teamId = currentTeam.id
    void (async () => {
      setLoading(true)
      // Promise.allSettled never rejects so a try/catch here would be
      // unreachable; failures are surfaced per-promise below.
      const [repliesRes, membersRes] = await Promise.allSettled([
        feedService.listReplies(teamId, itemId),
        teamService.getTeamMembers(teamId),
      ])
      // apiClient doesn't accept an AbortSignal yet — we only use this
      // flag to suppress stale setState, the request still completes.
      if (ctrl.signal.aborted) return
      if (repliesRes.status === 'fulfilled') {
        setReplies(repliesRes.value.replies)
      } else {
        handleError(repliesRes.reason, 'Failed to load replies')
      }
      if (membersRes.status === 'fulfilled') {
        setMembers(new Map(membersRes.value.map(m => [m.user_id, m])))
      }
      setLoading(false)
    })()
    return () => {
      ctrl.abort()
    }
  }, [currentTeam, itemId, handleError])

  // Auto-grow textarea — layout effect avoids a paint flash on paste.
  useLayoutEffect(() => {
    const el = draftRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${String(Math.min(el.scrollHeight, 200))}px`
  }, [draft])

  const handleSubmit = async () => {
    if (!currentTeam) return
    const trimmed = draft.trim()
    // Server-side limit is enforced by the API but we also gate here so a
    // stray paste of >MAX_REPLY_LENGTH chars doesn't go through and waste
    // a round-trip on certain rejection.
    if (trimmed === '' || trimmed.length > MAX_REPLY_LENGTH || submitting) {
      return
    }
    try {
      setSubmitting(true)
      const reply = await feedService.createReply(currentTeam.id, itemId, {
        content: trimmed,
        ai_assistant_name: USER_POST_ASSISTANT_NAME,
      })
      setReplies(prev => [...prev, reply])
      setDraft('')
      onReplyAdded?.()
    } catch (e) {
      handleError(e, 'Failed to post reply')
    } finally {
      setSubmitting(false)
    }
  }

  const showSkeleton = loading && replies.length === 0 && !initialReplyCount

  return (
    <div className="feed-thread">
      {showSkeleton && (
        <div className="text-xs text-muted-foreground">Loading replies…</div>
      )}

      {replies.map(reply => (
        <ReplyRow
          key={reply.id}
          reply={reply}
          member={members.get(reply.posted_by_user_id)}
        />
      ))}

      {/* Inline composer */}
      <ReplyComposer
        draft={draft}
        onChange={setDraft}
        onSubmit={() => {
          void handleSubmit()
        }}
        submitting={submitting}
        textareaRef={draftRef}
      />
    </div>
  )
}

interface ReplyRowProps {
  reply: FeedItemReply
  member: TeamMember | undefined
}

function ReplyRow({ reply, member }: ReplyRowProps) {
  const actor = resolveFeedActor(reply, member)
  return (
    <div className="feed-reply">
      <FeedActorAvatar actor={actor} size="sm" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-1.5 leading-tight">
          <span className="text-sm font-semibold text-foreground">
            {actor.displayName}
          </span>
          {actor.isAi && actor.aiAssistantName && (
            <span className="rounded border px-1.5 py-px text-xs text-muted-foreground">
              {actor.aiAssistantName}
            </span>
          )}
          <span aria-hidden="true" className="text-muted-foreground">
            ·
          </span>
          <span className="text-xs text-muted-foreground">
            {formatRelativeTime(reply.posted_at)}
          </span>
        </div>
        <p className="mt-0.5 whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground/90">
          {reply.content}
        </p>
      </div>
    </div>
  )
}

interface ReplyComposerProps {
  draft: string
  onChange: (value: string) => void
  onSubmit: () => void
  submitting: boolean
  textareaRef: React.RefObject<HTMLTextAreaElement | null>
}

function ReplyComposer({
  draft,
  onChange,
  onSubmit,
  submitting,
  textareaRef,
}: ReplyComposerProps) {
  const overLimit = draft.length > MAX_REPLY_LENGTH
  const canSubmit = draft.trim() !== '' && !overLimit && !submitting
  return (
    <div className="feed-reply">
      {/* Spacer to align with avatar column for visual rhythm */}
      <div
        className="size-7 shrink-0 rounded-full bg-muted/50"
        aria-hidden="true"
      />
      <div className="flex-1 rounded-[10px] border bg-background px-3 py-2 transition-colors focus-within:border-ring/60 focus-within:ring-2 focus-within:ring-ring/30">
        <textarea
          ref={textareaRef}
          value={draft}
          onChange={e => {
            onChange(e.target.value)
          }}
          onKeyDown={e => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              onSubmit()
            }
          }}
          placeholder="Reply…"
          rows={1}
          disabled={submitting}
          className="feed-composer-input block min-h-[22px] w-full resize-none bg-transparent py-1 text-sm leading-relaxed placeholder:text-muted-foreground"
        />
        <div className="mt-1.5 flex items-center justify-between gap-2 border-t pt-1.5">
          <span className="text-xs text-muted-foreground">
            <kbd className="rounded border px-1 font-mono text-xs">⌘</kbd>
            <span className="mx-1">+</span>
            <kbd className="rounded border px-1 font-mono text-xs">⏎</kbd>
            <span className="ml-1">to send</span>
          </span>
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                onChange('')
              }}
              disabled={submitting || draft === ''}
              className="h-7 px-2 text-xs"
            >
              Cancel
            </Button>
            <Button
              type="button"
              size="sm"
              onClick={onSubmit}
              disabled={!canSubmit}
              className="h-7 px-3 text-xs"
            >
              {submitting ? 'Posting…' : 'Reply'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
