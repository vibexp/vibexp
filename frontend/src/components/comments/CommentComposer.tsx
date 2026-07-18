import { useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { getErrorMessage } from '@/utils/errorHandling'

interface CommentComposerProps {
  /** Persists the comment; rejects to keep the draft with an inline error. */
  onSubmit: (content: string) => Promise<void>
  /** Called after a successful submit (parent closes the composer / edit row). */
  onSuccess?: () => void
  onCancel?: () => void
  submitLabel?: string
  placeholder?: string
  focusOnMount?: boolean
  initialValue?: string
}

/**
 * Inline compose/edit box shared by the "add comment" affordance and in-place
 * edit. Owns draft + submitting + error state so that on failure the editor
 * stays open with the draft intact and an inline error (design spec §5.4).
 * ⌘/Ctrl+Enter submits.
 */
export function CommentComposer({
  onSubmit,
  onSuccess,
  onCancel,
  submitLabel = 'Comment',
  placeholder = 'Add a comment…',
  focusOnMount = false,
  initialValue = '',
}: Readonly<CommentComposerProps>) {
  const [value, setValue] = useState(initialValue)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const trimmed = value.trim()

  // Focus on open via a ref rather than the `autoFocus` prop (jsx-a11y forbids
  // it); the composer only ever mounts in response to a deliberate user action
  // (Add comment / Edit), so moving focus into it is expected.
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    if (focusOnMount) {
      const el = textareaRef.current
      el?.focus()
      // Place the caret at the end of a pre-filled draft (in-place edit).
      el?.setSelectionRange(el.value.length, el.value.length)
    }
  }, [focusOnMount])

  const submit = async () => {
    if (!trimmed || submitting) return
    setSubmitting(true)
    setError(null)
    try {
      await onSubmit(trimmed)
      // Reset the draft so a still-mounted composer (the popup's add box) doesn't
      // keep the submitted text; the widget/edit-row cases unmount on onSuccess.
      setValue('')
      onSuccess?.()
    } catch (err) {
      setError(getErrorMessage(err, 'Failed to save comment'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="space-y-2">
      <Textarea
        ref={textareaRef}
        rows={3}
        className="min-h-[72px] resize-none"
        placeholder={placeholder}
        value={value}
        disabled={submitting}
        onChange={e => {
          setValue(e.target.value)
        }}
        onKeyDown={e => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault()
            void submit()
          }
        }}
      />
      {error && (
        <p className="text-destructive text-xs" role="alert">
          {error}
        </p>
      )}
      <div className="flex items-center justify-end gap-2">
        {onCancel && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={submitting}
            onClick={onCancel}
          >
            Cancel
          </Button>
        )}
        <Button
          type="button"
          size="sm"
          disabled={submitting || !trimmed}
          onClick={() => {
            void submit()
          }}
        >
          {submitting ? 'Saving…' : submitLabel}
        </Button>
      </div>
    </div>
  )
}
