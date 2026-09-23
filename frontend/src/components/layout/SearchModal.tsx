import { Search as SearchIcon } from 'lucide-react'
import { type KeyboardEvent, useEffect, useState } from 'react'
import { useNavigate } from 'react-router'

import { SearchModalAiSummary } from '@/components/layout/SearchModalAiSummary'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { useTeam } from '@/contexts/TeamContext'
import { aiSummarySettingsService } from '@/services/aiSummarySettingsService'

/**
 * Header search entry point: a ghost icon-button that opens a controlled
 * dialog with a multi-line query field and an explicit Search button.
 * Pressing Enter (without Shift) submits; Shift+Enter inserts a newline.
 *
 * A plain `Dialog` (not `CommandDialog`) is used deliberately — cmdk swallows
 * the Enter key, which would break the submit-on-Enter behavior.
 *
 * Once a query is typed, a collapsed AI Summary row offers a grounded answer
 * without leaving the dialog (#1079) — only when the team's AI Summary is
 * enabled and has a model provider, and never with an admin configure-hint
 * (that belongs on the full search page).
 */
export function SearchModal() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [value, setValue] = useState('')
  const { currentTeam } = useTeam()
  const teamId = currentTeam?.id
  const [aiSummaryAvailable, setAiSummaryAvailable] = useState(false)
  const query = value.trim()

  // Availability is read once per dialog open. Any failure hides the row.
  useEffect(() => {
    if (!open || !teamId) return
    let cancelled = false
    aiSummarySettingsService
      .getAISummarySettings(teamId)
      .then(settings => {
        if (!cancelled) {
          setAiSummaryAvailable(settings.available && settings.values.enabled)
        }
      })
      .catch(() => {
        if (!cancelled) setAiSummaryAvailable(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, teamId])

  const close = () => {
    setOpen(false)
    setValue('')
    setAiSummaryAvailable(false)
  }

  const submit = () => {
    if (!query) return
    close()
    void navigate(`/search?q=${encodeURIComponent(query)}`)
  }

  // Opens the full results with the AI Summary pre-expanded; `/search`
  // consumes the `summary=open` param.
  const seeFullResults = () => {
    close()
    void navigate(`/search?q=${encodeURIComponent(query)}&summary=open`)
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter submits; Shift+Enter inserts a newline.
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      submit()
    }
  }

  const handleOpenChange = (next: boolean) => {
    if (next) setOpen(true)
    else close()
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Search">
          <SearchIcon className="size-5" />
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Search</DialogTitle>
          <DialogDescription>
            Find prompts, artifacts, blueprints, and memories across your team.
          </DialogDescription>
        </DialogHeader>
        {/* Radix DialogContent autofocuses its first focusable child (this
            textarea) on open, so no explicit autoFocus prop is needed. The
            focus ring is suppressed in favour of the static border per design. */}
        <Textarea
          placeholder="Type your search…"
          aria-label="Search query"
          className="min-h-[120px] resize-none text-base focus-visible:ring-0 focus-visible:ring-offset-0"
          value={value}
          onChange={event => {
            setValue(event.target.value)
          }}
          onKeyDown={handleKeyDown}
        />
        {teamId && query && aiSummaryAvailable && (
          <SearchModalAiSummary
            teamId={teamId}
            query={query}
            onSeeFullResults={seeFullResults}
          />
        )}
        <DialogFooter>
          <Button onClick={submit} disabled={!query}>
            <SearchIcon className="mr-2 size-4" />
            Search
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
