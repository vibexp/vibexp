import { AlertCircle } from 'lucide-react'
import { useCallback, useRef, useState } from 'react'

import { PromptTemplateLoader } from '@/components/PromptTemplateLoader'
import { useAnalytics } from '@/hooks'
import type { Prompt } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

interface PromptMentionTextareaProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  rows?: number
  className?: string
  error?: string
  excludeCurrentPrompt?: string
  'data-testid'?: string
}

interface MentionState {
  isModalOpen: boolean
  cursorPosition: number
  startIndex: number
}

export function PromptMentionTextarea({
  value,
  onChange,
  placeholder = 'Write your prompt here...',
  rows = 20,
  className = '',
  error,
  excludeCurrentPrompt,
  'data-testid': testId,
}: PromptMentionTextareaProps) {
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
        ref={textareaRef}
        data-testid={testId}
        value={value}
        onChange={handleChange}
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
}
