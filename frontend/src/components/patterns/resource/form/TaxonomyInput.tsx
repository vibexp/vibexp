import { X } from 'lucide-react'
import { useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'

export interface TaxonomyInputProps {
  value: readonly string[]
  onChange: (next: string[]) => void
  placeholder?: string
  disabled?: boolean
  'data-testid'?: string
  'aria-label'?: string
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
  'data-testid': testId,
  'aria-label': ariaLabel,
}: Readonly<TaxonomyInputProps>) {
  const [draft, setDraft] = useState('')

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
        value={draft}
        disabled={disabled}
        placeholder={placeholder}
        data-testid={testId}
        aria-label={ariaLabel}
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
