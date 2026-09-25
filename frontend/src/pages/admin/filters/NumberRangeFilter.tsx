import type { KeyboardEvent } from 'react'
import { useId, useRef, useState } from 'react'

import { Input } from '@/components/ui/input'

import type { NumberRangeValue } from './advancedFilterParams'
import { parseCount } from './advancedFilterParams'

export interface NumberRangeFilterProps {
  /** Visible label, and the stem of the inputs' accessible names. */
  label: string
  value: NumberRangeValue
  onChange: (next: NumberRangeValue) => void
}

const toDraft = (value: number | undefined) =>
  value === undefined ? '' : String(value)

/**
 * An inclusive min/max pair of whole numbers ≥ 0, either bound optional.
 *
 * Commits on blur or Enter rather than on every keystroke: each commit is a
 * fetch plus a history `replace`, and a half-typed `1` on the way to `10` would
 * fire a pointless request. Invalid input (negative, fractional, min above max)
 * is marked `aria-invalid` with an inline message and never committed.
 */
export function NumberRangeFilter({
  label,
  value,
  onChange,
}: Readonly<NumberRangeFilterProps>) {
  const id = useId()
  const errorId = `${id}-error`
  const minRef = useRef<HTMLInputElement>(null)
  const maxRef = useRef<HTMLInputElement>(null)
  const [minDraft, setMinDraft] = useState(toDraft(value.min))
  const [maxDraft, setMaxDraft] = useState(toDraft(value.max))
  const [error, setError] = useState<{
    field: 'min' | 'max' | 'both'
    message: string
  } | null>(null)

  // Follow the committed value when it changes from outside (URL restore,
  // Clear), adjusting state during render rather than in an effect.
  const [committed, setCommitted] = useState(value)
  if (committed.min !== value.min || committed.max !== value.max) {
    setCommitted(value)
    setMinDraft(toDraft(value.min))
    setMaxDraft(toDraft(value.max))
    setError(null)
  }

  // A number input reports unparseable text (e.g. a half-typed `1e`) as an
  // empty value; `badInput` is what tells it apart from a cleared field, which
  // would otherwise silently drop the bound.
  const isBad = (raw: string, input: HTMLInputElement | null) =>
    input?.validity.badInput === true ||
    (raw.trim() !== '' && parseCount(raw.trim()) === undefined)

  const commit = () => {
    const min = parseCount(minDraft.trim())
    const max = parseCount(maxDraft.trim())
    if (isBad(minDraft, minRef.current)) {
      setError({ field: 'min', message: 'Whole number ≥ 0' })
      return
    }
    if (isBad(maxDraft, maxRef.current)) {
      setError({ field: 'max', message: 'Whole number ≥ 0' })
      return
    }
    if (min !== undefined && max !== undefined && min > max) {
      setError({ field: 'both', message: 'Min must be ≤ max' })
      return
    }
    setError(null)
    if (min !== value.min || max !== value.max) {
      onChange({ min, max })
    }
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') commit()
  }

  const minInvalid = error?.field === 'min' || error?.field === 'both'
  const maxInvalid = error?.field === 'max' || error?.field === 'both'

  return (
    <fieldset className="flex flex-col gap-1.5">
      <legend className="mb-1.5 text-sm font-medium leading-none">
        {label}
      </legend>
      <div className="flex items-center gap-2">
        <Input
          ref={minRef}
          type="number"
          inputMode="numeric"
          min={0}
          step={1}
          placeholder="Min"
          aria-label={`${label} minimum`}
          aria-invalid={minInvalid || undefined}
          aria-describedby={minInvalid ? errorId : undefined}
          value={minDraft}
          onChange={event => {
            setMinDraft(event.target.value)
          }}
          onBlur={commit}
          onKeyDown={handleKeyDown}
        />
        <span aria-hidden="true" className="text-muted-foreground">
          –
        </span>
        <Input
          ref={maxRef}
          type="number"
          inputMode="numeric"
          min={0}
          step={1}
          placeholder="Max"
          aria-label={`${label} maximum`}
          aria-invalid={maxInvalid || undefined}
          aria-describedby={maxInvalid ? errorId : undefined}
          value={maxDraft}
          onChange={event => {
            setMaxDraft(event.target.value)
          }}
          onBlur={commit}
          onKeyDown={handleKeyDown}
        />
      </div>
      {error && (
        <p id={errorId} role="alert" className="text-destructive text-xs">
          {error.message}
        </p>
      )}
    </fieldset>
  )
}
