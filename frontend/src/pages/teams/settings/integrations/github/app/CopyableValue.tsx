import { Check, Copy } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { toast } from '@/lib/toast'

export interface CopyableValueProps {
  label: string
  value: string
  /**
   * Renders the value in a wrapping monospace block rather than a single line.
   * Used for the webhook secret, which is long and has no meaningful prefix to
   * truncate to.
   */
  multiline?: boolean
}

/**
 * A read-only value with a copy button.
 *
 * The value is rendered as text rather than a disabled input so it can wrap and
 * be selected normally. Nothing here is logged: these are the two values an
 * admin must carry over to GitHub, and one of them is a secret.
 */
export function CopyableValue({
  label,
  value,
  multiline = false,
}: CopyableValueProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    void navigator.clipboard
      .writeText(value)
      .then(() => {
        setCopied(true)
        setTimeout(() => {
          setCopied(false)
        }, 2000)
      })
      .catch(() => {
        // A clipboard denied by permissions must not look like a successful
        // copy — the admin would paste the previous clipboard contents into
        // GitHub and get a signature mismatch with no clue why.
        toast.error(`Could not copy the ${label.toLowerCase()}`, {
          description: 'Select the value and copy it manually.',
        })
      })
  }

  return (
    <div className="space-y-1">
      <p className="text-sm font-medium">{label}</p>
      <div className="bg-muted flex items-start gap-2 rounded-md border p-2">
        <code
          className={
            multiline
              ? 'flex-1 font-mono text-xs break-all'
              : 'flex-1 truncate font-mono text-xs'
          }
          data-testid={`copyable-${label.toLowerCase().replace(/\s+/g, '-')}`}
        >
          {value}
        </code>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={handleCopy}
          aria-label={`Copy ${label}`}
        >
          {copied ? (
            <Check className="h-3.5 w-3.5" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
        </Button>
      </div>
    </div>
  )
}
