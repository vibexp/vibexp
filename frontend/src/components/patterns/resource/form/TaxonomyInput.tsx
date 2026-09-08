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
    if (next === '' || value.includes(next) || atCap) {
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
