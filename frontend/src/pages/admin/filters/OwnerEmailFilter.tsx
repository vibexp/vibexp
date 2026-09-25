import { useId, useState } from 'react'

import { Input } from '@/components/ui/input'
import { ownerEmailParam } from '@/pages/admin/filters/advancedFilterParams'

export interface OwnerEmailFilterProps {
  /**
   * What the published spec matches, e.g. "Primary owner email" (teams:
   * `teams.owner_id`) or "Creator email" (projects: `projects.user_id`).
   */
  label: string
  value: string
  onChange: (value: string) => void
}

/**
 * Owner email, committed on blur or Enter like the range inputs, so typing an
 * address does not fire a request per keystroke.
 *
 * The API matches the address exactly and rejects a malformed one with a 400 —
 * so a half-typed address is marked invalid and never committed, rather than
 * replacing the list with an error that a reload would repeat. A malformed value
 * restored from the URL is shown as invalid too (`ownerEmailParam` drops it from
 * the request).
 *
 * The page remounts it through a `key` on Clear: a rejected draft never reached
 * the URL, so the URL alone cannot tell it to reset.
 */
export function OwnerEmailFilter({
  label,
  value,
  onChange,
}: Readonly<OwnerEmailFilterProps>) {
  const id = useId()
  const errorId = `${id}-error`
  const isInvalid = (raw: string) =>
    raw.trim() !== '' && ownerEmailParam(raw) === undefined
  const [draft, setDraft] = useState(value)
  const [invalid, setInvalid] = useState(() => isInvalid(value))

  // Follow the committed value when it changes from outside (URL restore,
  // Clear), adjusting state during render rather than in an effect.
  const [committed, setCommitted] = useState(value)
  if (committed !== value) {
    setCommitted(value)
    setDraft(value)
    setInvalid(isInvalid(value))
  }

  const commit = () => {
    const next = draft.trim()
    if (isInvalid(next)) {
      setInvalid(true)
      return
    }
    setInvalid(false)
    if (next !== value) onChange(next)
  }

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-medium leading-none">
        {label}
      </label>
      <Input
        id={id}
        type="email"
        placeholder="owner@example.com"
        value={draft}
        onChange={event => {
          setDraft(event.target.value)
        }}
        aria-invalid={invalid || undefined}
        aria-describedby={invalid ? errorId : undefined}
        onBlur={commit}
        onKeyDown={event => {
          if (event.key === 'Enter') commit()
        }}
      />
      {invalid && (
        <p id={errorId} role="alert" className="text-destructive text-xs">
          Enter a full email address
        </p>
      )}
    </div>
  )
}
