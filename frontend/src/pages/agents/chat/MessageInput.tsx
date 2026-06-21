import { Send } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

interface MessageInputProps {
  value: string
  onChange: (value: string) => void
  onSend: () => void
  disabled: boolean
}

export function MessageInput({
  value,
  onChange,
  onSend,
  disabled,
}: MessageInputProps) {
  return (
    <div className="bg-background border-t p-4">
      <div className="flex items-end gap-2">
        <Textarea
          rows={2}
          className="min-h-[60px] flex-1 resize-none"
          placeholder="Type your message… (Enter to send, Shift+Enter for new line)"
          value={value}
          onChange={e => {
            onChange(e.target.value)
          }}
          onKeyDown={e => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              if (!disabled && value.trim()) {
                onSend()
              }
            }
          }}
          disabled={disabled}
        />
        <Button
          onClick={onSend}
          disabled={disabled || !value.trim()}
          size="icon"
          aria-label="Send message"
        >
          <Send className="size-4" />
        </Button>
      </div>
    </div>
  )
}
