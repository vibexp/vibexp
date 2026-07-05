import {
  Archive,
  ArchiveRestore,
  ArrowRight,
  Copy,
  MessageSquare,
  Share2,
  Trash2,
} from 'lucide-react'
import { memo, useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatRelativeTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import { FeedActorAvatar, resolveFeedActor } from '@/pages/feeds/feedActor'
import { InlineReplyThread } from '@/pages/feeds/InlineReplyThread'
import type { FeedItem } from '@/services/feedService'
import type { TeamMember } from '@/types/team'

interface FeedItemCardProps {
  item: FeedItem
  feedName?: string
  projectName?: string
  replyCount?: number
  member?: TeamMember
  onArchive?: (item: FeedItem) => Promise<void>
  onUnarchive?: (item: FeedItem) => Promise<void>
  onDelete?: (item: FeedItem) => void
}

function ScopeBadge({ children }: { children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1 rounded border bg-secondary px-1.5 py-0.5 text-xs font-medium text-secondary-foreground">
      {children}
    </span>
  )
}

interface FootButtonProps {
  icon?: React.ComponentType<{ className?: string }>
  label: string
  ariaLabel?: string
  onClick?: () => void
  disabled?: boolean
  /**
   * Set only when the button is a true toggle (Reply expand/collapse).
   * Other action buttons (Copy, Share, Archive) leave this undefined so
   * they don't get an incorrect `aria-pressed` from screen readers.
   */
  toggled?: boolean
  count?: number
  className?: string
}

function FootButton({
  icon: Icon,
  label,
  ariaLabel,
  onClick,
  disabled,
  toggled,
  count,
  className,
}: FootButtonProps) {
  return (
    <button
      type="button"
      aria-label={ariaLabel ?? label}
      aria-pressed={toggled}
      onClick={e => {
        e.stopPropagation()
        onClick?.()
      }}
      disabled={disabled}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50',
        toggled
          ? 'bg-accent text-foreground'
          : 'text-muted-foreground hover:bg-accent hover:text-foreground',
        className
      )}
    >
      {Icon && <Icon className="size-3.5" />}
      <span>{label}</span>
      {count !== undefined && count > 0 && (
        <span className="ml-0.5 inline-flex items-center rounded px-1 text-xs tabular-nums">
          {count}
        </span>
      )}
    </button>
  )
}

function FeedItemCardImpl({
  item,
  feedName,
  projectName,
  replyCount: replyCountProp,
  member,
  onArchive,
  onUnarchive,
  onDelete,
}: FeedItemCardProps) {
  const isArchived = !!item.archived_at
  const actor = resolveFeedActor(item, member)

  // Track replyCount in local state so we can optimistically increment
  // when the user posts a reply, but stay in sync with the parent prop
  // (e.g. after a refresh/refetch). Latest server count wins, plus any
  // optimistic delta accumulated since the last prop change.
  const [optimisticDelta, setOptimisticDelta] = useState<number>(0)
  const [lastSeenProp, setLastSeenProp] = useState<number | undefined>(
    replyCountProp
  )
  if (lastSeenProp !== replyCountProp) {
    // Reset optimistic delta whenever the parent ships a fresh count.
    setLastSeenProp(replyCountProp)
    setOptimisticDelta(0)
  }
  const replyCount = (replyCountProp ?? 0) + optimisticDelta

  const [threadOpen, setThreadOpen] = useState<boolean>(false)
  const [bodyExpanded, setBodyExpanded] = useState<boolean>(false)

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(
        `${item.title}\n\n${item.content || item.excerpt || ''}`
      )
    } catch {
      // Best effort: silently ignore clipboard failures (denied
      // permission, insecure context, etc.)
    }
  }, [item.title, item.content, item.excerpt])

  const detailHref = useMemo(
    () => `/feed-items/${encodeURIComponent(item.id)}`,
    [item.id]
  )

  // Use excerpt (server-truncated) as the feed body. Fall back to content
  // only when no excerpt exists (older items, edge cases).
  const bodyText = item.excerpt || item.content || ''
  // Heuristic: clamp/expand only useful when the body has more than ~3 lines.
  // The regex is a cheap stand-in for `split('\n').length > 3` to avoid an
  // allocation on every render.
  const isLong = bodyText.length > 240 || /\n[^\n]*\n[^\n]*\n/.test(bodyText)

  const handleReplyAdded = useCallback(() => {
    setOptimisticDelta(d => d + 1)
  }, [])

  return (
    <article className="group rounded-xl border bg-card px-4 py-4 shadow-sm transition-shadow hover:shadow-md">
      {/* Header: avatar + meta + top actions */}
      <div className="flex items-start gap-2.5">
        <FeedActorAvatar actor={actor} size="md" />

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-sm">
            <span className="font-semibold text-foreground">
              {actor.displayName}
            </span>
            {actor.isAi && (
              <Badge variant="outline" className="text-xs">
                AI
              </Badge>
            )}
            <span aria-hidden="true" className="text-muted-foreground">
              ·
            </span>
            <Link
              to={detailHref}
              aria-label={`Open feed item: ${item.title}`}
              className="text-xs text-muted-foreground hover:text-foreground hover:underline underline-offset-2"
            >
              {formatRelativeTime(item.posted_at)}
            </Link>
            {isArchived && (
              <>
                <span aria-hidden="true" className="text-muted-foreground">
                  ·
                </span>
                <span className="text-xs italic text-muted-foreground">
                  Archived
                </span>
              </>
            )}
          </div>
          {((item.project_id && projectName) ?? feedName) && (
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              {item.project_id && projectName && (
                <ScopeBadge>{projectName}</ScopeBadge>
              )}
              {feedName && <ScopeBadge>{feedName}</ScopeBadge>}
            </div>
          )}
        </div>

        {/* Top-right actions — destructive only, appear on hover */}
        <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
          {onDelete && (
            <Button
              variant="ghost"
              size="icon"
              aria-label="Delete"
              className="size-7"
              onClick={e => {
                e.stopPropagation()
                onDelete(item)
              }}
            >
              <Trash2 className="size-3.5 text-destructive" />
            </Button>
          )}
        </div>
      </div>

      {/* Body — indented to align under name (avatar 36 + gap 10 = 46) */}
      <div className="mt-1.5 pl-[46px]">
        <Link
          to={detailHref}
          className="mb-1 inline-block text-base font-semibold leading-snug tracking-tight text-foreground hover:underline underline-offset-[3px] decoration-1"
        >
          {item.title}
        </Link>
        <p
          className={cn(
            'whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground/85',
            isLong && !bodyExpanded && 'line-clamp-3'
          )}
        >
          {bodyText}
        </p>
        {isLong && (
          <button
            type="button"
            onClick={() => {
              setBodyExpanded(v => !v)
            }}
            className="mt-1 text-xs font-medium text-muted-foreground hover:text-foreground hover:underline underline-offset-2"
          >
            {bodyExpanded ? 'Show less' : 'Show more'}
          </button>
        )}
        {replyCount > 0 && (
          <div className="mt-2">
            <Link
              to={detailHref}
              className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground hover:underline underline-offset-2"
            >
              View full post
              <ArrowRight className="size-3" />
            </Link>
          </div>
        )}
      </div>

      {/* Footer action bar */}
      <div className="mt-2.5 flex items-center gap-1 pl-[46px]">
        <FootButton
          icon={Copy}
          label="Copy"
          onClick={() => {
            void handleCopy()
          }}
        />
        <FootButton
          icon={MessageSquare}
          label="Reply"
          toggled={threadOpen}
          count={replyCount}
          onClick={() => {
            setThreadOpen(v => !v)
          }}
        />
        <FootButton icon={Share2} label="Share" disabled />
        {!isArchived && onArchive && (
          <FootButton
            icon={Archive}
            label="Archive"
            onClick={() => {
              void onArchive(item)
            }}
          />
        )}
        {isArchived && onUnarchive && (
          <FootButton
            icon={ArchiveRestore}
            label="Unarchive"
            onClick={() => {
              void onUnarchive(item)
            }}
          />
        )}
        {threadOpen && replyCount > 0 && (
          <FootButton
            label={
              replyCount === 1
                ? 'Hide 1 reply'
                : `Hide ${String(replyCount)} replies`
            }
            onClick={() => {
              setThreadOpen(false)
            }}
            className="ml-auto"
          />
        )}
      </div>

      {threadOpen && (
        <div className="pl-[46px]">
          <InlineReplyThread
            itemId={item.id}
            initialReplyCount={replyCount}
            onReplyAdded={handleReplyAdded}
          />
        </div>
      )}
    </article>
  )
}

export const FeedItemCard = memo(FeedItemCardImpl)
