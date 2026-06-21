import { Search as SearchIcon } from 'lucide-react'
import { type KeyboardEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'

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

/**
 * Header search entry point: a ghost icon-button that opens a controlled
 * dialog with a multi-line query field and an explicit Search button.
 * Pressing Enter (without Shift) submits; Shift+Enter inserts a newline.
 *
 * A plain `Dialog` (not `CommandDialog`) is used deliberately — cmdk swallows
 * the Enter key, which would break the submit-on-Enter behavior.
 */
export function SearchModal() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [value, setValue] = useState('')

  const submit = () => {
    const query = value.trim()
    if (!query) return
    setOpen(false)
    setValue('')
    void navigate(`/search?q=${encodeURIComponent(query)}`)
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter submits; Shift+Enter inserts a newline.
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      submit()
    }
  }

  const handleOpenChange = (next: boolean) => {
    setOpen(next)
    if (!next) setValue('')
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
        <DialogFooter>
          <Button onClick={submit} disabled={!value.trim()}>
            <SearchIcon className="mr-2 size-4" />
            Search
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
