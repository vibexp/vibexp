import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { cn } from '@/lib/utils'

interface ReplyComposerProps {
  /** Persists the reply; a rejection keeps the draft and reports the error. */
  onSubmit: (content: string) => Promise<void>
  className?: string
}

/**
 * Reply compose box shared by the replies panel and its "all replies" popup.
 * Owns the draft and submitting state; Enter submits, Shift+Enter adds a line.
 */
export function ReplyComposer({
  onSubmit,
  className,
}: Readonly<ReplyComposerProps>) {
  const { handleError } = useErrorHandler()
  const [content, setContent] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const submit = async () => {
    if (!content.trim()) return
    try {
      setSubmitting(true)
      await onSubmit(content.trim())
      setContent('')
    } catch (err) {
      handleError(err, 'Failed to post reply')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className={cn('flex items-end gap-2', className)}>
      <Textarea
        rows={2}
        className="min-h-[60px] flex-1 resize-none"
        placeholder="Write a reply..."
        value={content}
        onChange={e => {
          setContent(e.target.value)
        }}
        onKeyDown={e => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault()
            void submit()
          }
        }}
        disabled={submitting}
      />
      <Button
        onClick={() => {
          void submit()
        }}
        disabled={submitting || !content.trim()}
        size="sm"
      >
        {submitting ? 'Posting...' : 'Reply'}
      </Button>
    </div>
  )
}
