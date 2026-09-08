import { AlertCircle } from 'lucide-react'
import { forwardRef, useCallback, useRef, useState } from 'react'

import { PromptTemplateLoader } from '@/components/PromptTemplateLoader'
import { useAnalytics } from '@/hooks'
import type { Prompt } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

interface PromptMentionTextareaProps {
  value: string
  onChange: (value: string) => void
  onBlur?: () => void
  placeholder?: string
  rows?: number
  className?: string
  error?: string
  disabled?: boolean
  excludeCurrentPrompt?: string
  'data-testid'?: string
  /**
   * The accessible name, and the ids shadcn's `FormControl` (a Radix `Slot`)
   * clones onto its child. They must reach the `<textarea>` itself: a
   * component in between that does not forward them leaves every `FormLabel`
   * `htmlFor` dangling and every validation message unannounced, silently.
   */
  'aria-label'?: string
  id?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean
}

interface MentionState {
  isModalOpen: boolean
  cursorPosition: number
  startIndex: number
}

/**
 * A markdown textarea that opens the prompt picker when the user types `@`.
 *
 * Consumed through `ResourceBodyEditor`'s `mentions` extension (#914), which
 * is why it forwards a ref and accepts the form-control props: it is the leaf
 * `<textarea>` of a generated form's body field, so react-hook-form's `ref` /
 * `onBlur` and the `FormControl` slot ids have to land on it.
 */
export const PromptMentionTextarea = forwardRef<
  HTMLTextAreaElement,
  Readonly<PromptMentionTextareaProps>
>(function PromptMentionTextarea(
  {
    value,
    onChange,
    onBlur,
    placeholder = 'Write your prompt here...',
    rows = 20,
    className = '',
    error,
    disabled,
    excludeCurrentPrompt,
    'data-testid': testId,
    'aria-label': ariaLabel,
    id,
    'aria-describedby': describedBy,
    'aria-invalid': invalid,
  },
  forwardedRef
) {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const { trackEvent } = useAnalytics()
  const [mentionState, setMentionState] = useState<MentionState>({
    isModalOpen: false,
    cursorPosition: 0,
    startIndex: 0,
  })

  // Handle text change and detect @ mentions
  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const newValue = e.target.value
      const cursorPosition = e.target.selectionStart

      onChange(newValue)

      // Check if user just typed @ to open modal
      const lastChar = newValue[cursorPosition - 1]
      if (lastChar === '@') {
        setMentionState({
          isModalOpen: true,
          cursorPosition,
          startIndex: cursorPosition - 1, // Index of the @ symbol
        })

        // Track reusable prompt modal triggered event
        trackEvent({
          event: ANALYTICS_EVENTS.REUSABLE_PROMPT_MODAL_TRIGGERED,
          properties: {
            action_context: 'modal_trigger',
          },
        })
      }
    },
    [onChange, trackEvent]
  )

  // Close the modal
  const closeMentionModal = useCallback(() => {
    setMentionState(prev => ({ ...prev, isModalOpen: false }))
  }, [])

  // Handle prompt selection from modal
  const handlePromptSelect = useCallback(
    (prompt: Prompt) => {
      if (!textareaRef.current) return

      // Replace the @ symbol with @prompt-slug
      const beforeMention = value.substring(0, mentionState.startIndex)
      const afterCursor = value.substring(mentionState.cursorPosition)
      const newValue = `${beforeMention}@${prompt.slug}${afterCursor}`

      onChange(newValue)
      closeMentionModal()

      // Set cursor position after the mention
      setTimeout(() => {
        if (textareaRef.current) {
          const newCursorPosition =
            beforeMention.length + prompt.slug.length + 1
          textareaRef.current.setSelectionRange(
            newCursorPosition,
            newCursorPosition
          )
          textareaRef.current.focus()
        }
      }, 0)
    },
    [value, mentionState, onChange, closeMentionModal]
  )

  return (
    <div className="relative">
      <textarea
        // Both refs: the caller's (react-hook-form registers the leaf) and the
        // internal one the mention insertion needs to restore the caret.
        ref={node => {
          textareaRef.current = node
          if (typeof forwardedRef === 'function') forwardedRef(node)
          else if (forwardedRef) forwardedRef.current = node
        }}
        id={id}
        aria-label={ariaLabel}
        aria-describedby={describedBy}
        aria-invalid={invalid}
        data-testid={testId}
        value={value}
        onChange={handleChange}
        onBlur={onBlur}
        disabled={disabled}
        placeholder={placeholder}
        rows={rows}
        className={`w-full px-4 py-3 border rounded-lg focus:ring-2 focus:ring-ring focus:border-transparent font-mono text-sm resize-y ${
          error ? 'border-destructive' : 'border-input'
        } ${className}`}
      />

      {error && (
        <p className="mt-1 text-sm text-destructive flex items-center">
          <AlertCircle className="h-4 w-4 mr-1" />
          {error}
        </p>
      )}

      {/* Prompt Selection Modal */}
      <PromptTemplateLoader
        isOpen={mentionState.isModalOpen}
        onClose={closeMentionModal}
        onSelectPrompt={handlePromptSelect}
        excludeCurrentPrompt={excludeCurrentPrompt}
      />
    </div>
  )
})
