import { Hash, Image as ImageIcon, Paperclip } from 'lucide-react'
import { useLayoutEffect, useMemo, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useTeam } from '@/contexts/TeamContext'
import { useAuth } from '@/contexts/useAuth'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { assistantColor, assistantInitial } from '@/lib/avatar'
import { cn } from '@/lib/utils'
import { USER_POST_ASSISTANT_NAME } from '@/pages/feeds/feedActor'
import { feedService } from '@/services/feedService'
import type { Project } from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'

export { USER_POST_ASSISTANT_NAME }
const MAX_TITLE_LENGTH = 255
const MAX_CONTENT_BYTES = 200 * 1024 // 200 KB
const MAX_CONTENT_CHARS = 4000
const NO_PROJECT = '__none__'

const encoder = new TextEncoder()

interface FeedPostComposerProps {
  feedId: string
  projects: Project[]
  onPosted: () => void
}

export function FeedPostComposer({
  feedId,
  projects,
  onPosted,
}: FeedPostComposerProps) {
  const { currentTeam } = useTeam()
  const { user } = useAuth()
  const { handleError } = useErrorHandler()
  const { showSuccess } = useAlerts()
  const { trackEvent } = useAnalytics()

  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [projectId, setProjectId] = useState<string>(NO_PROJECT)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [touched, setTouched] = useState(false)
  const [expanded, setExpanded] = useState(false)

  const contentRef = useRef<HTMLTextAreaElement | null>(null)

  const titleTrimmed = title.trim()
  const contentTrimmed = content.trim()

  const contentBytes = useMemo(
    () => encoder.encode(contentTrimmed).byteLength,
    [contentTrimmed]
  )

  const titleError =
    touched && titleTrimmed === '' ? 'Title is required' : undefined
  const contentError =
    touched && contentTrimmed === '' ? 'Content is required' : undefined
  const contentTooLargeError =
    contentBytes > MAX_CONTENT_BYTES
      ? 'Content exceeds maximum size (200 KB)'
      : undefined

  const isValid =
    titleTrimmed !== '' &&
    contentTrimmed !== '' &&
    contentBytes <= MAX_CONTENT_BYTES
  const isDisabled = !isValid || isSubmitting || !feedId
  const hasAnyInput = title !== '' || content !== '' || projectId !== NO_PROJECT

  // Auto-grow textarea. useLayoutEffect avoids a paint flash on long
  // pastes — the height adjustment runs before the browser paints.
  useLayoutEffect(() => {
    const el = contentRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${String(Math.min(el.scrollHeight, 320))}px`
  }, [content, expanded])

  const handleExpand = () => {
    if (!expanded) setExpanded(true)
  }

  const handleCollapse = () => {
    if (hasAnyInput || isSubmitting) return
    setExpanded(false)
    setTouched(false)
  }

  const handleCancel = () => {
    setTitle('')
    setContent('')
    setProjectId(NO_PROJECT)
    setTouched(false)
    setExpanded(false)
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setTouched(true)

    if (!isValid || !currentTeam || !feedId) return

    try {
      setIsSubmitting(true)
      const item = await feedService.createFeedItem(currentTeam.id, feedId, {
        title: titleTrimmed,
        content: contentTrimmed,
        ai_assistant_name: USER_POST_ASSISTANT_NAME,
        project_id: projectId !== NO_PROJECT ? projectId : undefined,
      })

      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_POSTED,
        properties: { feed_id: feedId, feed_item_id: item.id },
      })

      setTitle('')
      setContent('')
      setProjectId(NO_PROJECT)
      setTouched(false)
      setExpanded(false)

      showSuccess('Feed item posted successfully', 'Success')
      onPosted()
    } catch (err) {
      handleError(err, 'Failed to post feed item')
    } finally {
      setIsSubmitting(false)
    }
  }

  const userName = user?.name ?? user?.email ?? 'You'
  const initial = assistantInitial(userName)
  const avatarBg = assistantColor(userName)

  const charCount = content.length
  const overChar = charCount > MAX_CONTENT_CHARS

  return (
    <form
      onSubmit={e => {
        void handleSubmit(e)
      }}
      onBlur={e => {
        // Don't collapse if focus is moving inside the form, or to a
        // Radix portal that the form opened (Select dropdown items live
        // in `body`, not inside the form subtree). Also skip when the
        // user has typed anything — `handleCollapse` re-checks this
        // before actually collapsing.
        const next = e.relatedTarget
        if (e.currentTarget.contains(next) || hasAnyInput) {
          return
        }
        if (
          next instanceof HTMLElement &&
          next.closest('[data-radix-popper-content-wrapper]')
        ) {
          return
        }
        handleCollapse()
      }}
      aria-busy={isSubmitting}
      className="rounded-xl border bg-card p-4 shadow-sm transition-shadow focus-within:border-ring/60 focus-within:ring-2 focus-within:ring-ring/20"
    >
      <div className="flex gap-3">
        {/* Avatar */}
        <div
          className={cn(
            'flex size-9 shrink-0 items-center justify-center rounded-full font-semibold text-sm text-white',
            avatarBg
          )}
          aria-hidden="true"
        >
          {initial}
        </div>

        {/* Body column */}
        <div className="flex flex-1 flex-col gap-2 min-w-0">
          {/* Title input — always visible. Doubles as collapsed CTA. */}
          <input
            type="text"
            placeholder={expanded ? 'Title' : 'Share an update…'}
            value={title}
            maxLength={MAX_TITLE_LENGTH}
            onFocus={handleExpand}
            onChange={e => {
              setTitle(e.target.value)
            }}
            disabled={isSubmitting}
            aria-label="Post title"
            aria-invalid={!!titleError}
            aria-describedby={titleError ? 'post-title-error' : undefined}
            className={cn(
              'feed-composer-input w-full bg-transparent py-1.5 text-base font-semibold text-foreground placeholder:font-medium placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-60',
              !expanded && 'font-medium text-sm'
            )}
          />

          {expanded && (
            <>
              {titleError && (
                <p
                  id="post-title-error"
                  className="text-xs text-destructive"
                  role="alert"
                >
                  {titleError}
                </p>
              )}
              <textarea
                ref={contentRef}
                placeholder="What's on your mind? Paste a snippet, drop a thought, log what you just did…"
                value={content}
                rows={3}
                onChange={e => {
                  setContent(e.target.value)
                }}
                disabled={isSubmitting}
                aria-label="Post content"
                aria-invalid={!!contentError || !!contentTooLargeError}
                aria-describedby={
                  contentError
                    ? 'post-content-error'
                    : contentTooLargeError
                      ? 'post-content-size-error'
                      : undefined
                }
                className="feed-composer-input block min-h-[60px] w-full resize-none bg-transparent text-sm leading-relaxed text-foreground placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-60"
              />
              {contentError && (
                <p
                  id="post-content-error"
                  className="text-xs text-destructive"
                  role="alert"
                >
                  {contentError}
                </p>
              )}
              {contentTooLargeError && (
                <p
                  id="post-content-size-error"
                  className="text-xs text-destructive"
                  role="alert"
                >
                  {contentTooLargeError}
                </p>
              )}

              {/* Toolbar — separated by border-top */}
              <div className="mt-2 flex items-center justify-between gap-2 border-t pt-3">
                <div className="flex items-center gap-1">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    disabled
                    aria-label="Attach file"
                    className="size-8 text-muted-foreground hover:text-foreground"
                  >
                    <Paperclip className="size-4" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    disabled
                    aria-label="Image"
                    className="size-8 text-muted-foreground hover:text-foreground"
                  >
                    <ImageIcon className="size-4" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    disabled
                    aria-label="Tag"
                    className="size-8 text-muted-foreground hover:text-foreground"
                  >
                    <Hash className="size-4" />
                  </Button>
                  <Select
                    value={projectId}
                    onValueChange={setProjectId}
                    disabled={isSubmitting}
                  >
                    <SelectTrigger
                      className="ml-1 h-8 w-auto min-w-[7rem] gap-1.5 px-2 text-xs"
                      aria-label="Select project (optional)"
                    >
                      <SelectValue placeholder="No project" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={NO_PROJECT}>No project</SelectItem>
                      {projects.map(project => (
                        <SelectItem key={project.id} value={project.id}>
                          {project.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className="flex items-center gap-2">
                  <span
                    className={cn(
                      'text-xs tabular-nums',
                      overChar ? 'text-destructive' : 'text-muted-foreground'
                    )}
                  >
                    {charCount} / {MAX_CONTENT_CHARS}
                  </span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={handleCancel}
                    disabled={isSubmitting}
                    className="h-8 px-3 text-xs"
                  >
                    Cancel
                  </Button>
                  <Button
                    type="submit"
                    size="sm"
                    disabled={isDisabled}
                    className="h-8 px-4 text-xs"
                  >
                    {isSubmitting ? 'Posting…' : 'Post'}
                  </Button>
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </form>
  )
}
