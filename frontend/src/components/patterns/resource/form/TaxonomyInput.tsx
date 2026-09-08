import { X } from 'lucide-react'
import { useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'

export interface TaxonomyInputProps {
  value: readonly string[]
  onChange: (next: string[]) => void
  placeholder?: string
  disabled?: boolean
  /** Stops adding past the API's own cap, and says so. */
  maxItems?: number
  /**
   * Max characters per entry, enforced natively on the draft input.
   *
   * The schema caps entries too, but a zod error inside an array lands at
   * `labels.0` and react-hook-form nests it — `FormMessage` reads
   * `error.message` off the array node, finds none, and renders nothing. An
   * over-long entry would therefore block Save with no visible reason at all,
   * so the control must make one impossible to type in the first place.
   */
  maxEntryLength?: number
  id?: string
  'data-testid'?: string
  'aria-label'?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean
}

/**
 * A chip editor over a list of strings — prompt labels, and whatever taxonomy
 * a user-defined resource declares.
 *
 * `TaxonomyChips` renders the same values read-only on the detail page; this
 * is its editable counterpart, which existed twice inline (the prompt editor's
 * label row and the memory form's tag card) and nowhere reusable.
 */
export function TaxonomyInput({
  value,
  onChange,
  placeholder,
  disabled = false,
  maxItems,
  maxEntryLength,
  id,
  'data-testid': testId,
  'aria-label': ariaLabel,
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
}: Readonly<TaxonomyInputProps>) {
  const [draft, setDraft] = useState('')
  const atCap = maxItems !== undefined && value.length >= maxItems

  const commit = () => {
    const next = draft.trim()
    if (next === '' || value.includes(next)) {
      setDraft('')
      return
    }
    onChange([...value, next])
    setDraft('')
  }

  return (
    <div className="space-y-2">
      <Input
        id={id}
        value={draft}
        disabled={disabled || atCap}
        placeholder={placeholder}
        maxLength={maxEntryLength}
        data-testid={testId}
        aria-label={ariaLabel}
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid}
        onChange={event => {
          setDraft(event.target.value)
        }}
        onKeyDown={event => {
          // Comma as well as Enter: a list typed as "a, b, c" is the shape
          // people reach for, and Enter alone would submit the form.
          if (event.key !== 'Enter' && event.key !== ',') return
          event.preventDefault()
          commit()
        }}
      />
      {maxItems !== undefined && (
        <p className="text-muted-foreground text-xs">
          {String(value.length)}/{String(maxItems)}
        </p>
      )}
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {value.map(entry => (
            <Badge key={entry} variant="secondary" className="gap-1">
              {entry}
              <button
                type="button"
                disabled={disabled}
                aria-label={`Remove ${entry}`}
                onClick={() => {
                  onChange(value.filter(candidate => candidate !== entry))
                }}
              >
                <X className="size-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}
    </div>
  )
}
