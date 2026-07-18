import { Check, Copy } from 'lucide-react'
import { useState } from 'react'

import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import type { Team } from '@/services/teamService'

interface TeamIdentifiersProps {
  teams: Team[]
}

/** Two-letter initials from a team name, used for the card avatar. */
function initialsOf(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 0) return '?'
  // Iterate by code point so a leading emoji/multi-byte char isn't split.
  if (words.length === 1) {
    return Array.from(words[0]).slice(0, 2).join('').toUpperCase()
  }
  const first = Array.from(words[0])[0]
  const last = Array.from(words.at(-1) ?? '')[0]
  return (first + last).toUpperCase()
}

function CopyField({
  value,
  label,
}: Readonly<{ value: string; label: string }>) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    navigator.clipboard
      .writeText(value)
      .then(() => {
        setCopied(true)
        setTimeout(() => {
          setCopied(false)
        }, 1500)
      })
      .catch((err: unknown) => {
        console.error('Copy failed:', err)
      })
  }

  return (
    <div className="border-input bg-background flex min-w-0 items-center overflow-hidden rounded-md border">
      <code
        title={value}
        className="text-foreground min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap px-3 py-2 font-mono text-sm"
      >
        {value}
      </code>
      <button
        type="button"
        onClick={handleCopy}
        title="Copy"
        aria-label={`Copy ${label} for ${value}`}
        className={cn(
          'bg-muted/50 hover:bg-accent hover:text-foreground grid w-[38px] shrink-0 cursor-pointer place-items-center self-stretch border-l transition-colors',
          copied ? 'text-success' : 'text-muted-foreground'
        )}
      >
        {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
      </button>
    </div>
  )
}

function FieldLabel({ children }: Readonly<{ children: string }>) {
  return (
    <span className="text-muted-foreground text-xs font-semibold uppercase tracking-wide">
      {children}
    </span>
  )
}

/**
 * Lists each team's UUID and slug as individually copyable identifiers. With the
 * team-agnostic MCP endpoint, the AI client must pass a team_id (UUID or slug)
 * per tool call, so users need to hand one of these identifiers to their agent.
 */
export function TeamIdentifiers({ teams }: Readonly<TeamIdentifiersProps>) {
  if (teams.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        You don&rsquo;t belong to any teams yet — create or join a team to get a
        team identifier for MCP tools.
      </p>
    )
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {teams.map(team => (
        <Card key={team.id} className="min-w-0 p-[18px]">
          <div className="mb-4 flex items-center gap-[11px]">
            <span className="bg-secondary text-secondary-foreground grid size-[34px] shrink-0 place-items-center rounded-md text-sm font-bold">
              {initialsOf(team.name)}
            </span>
            <span className="text-base font-semibold">{team.name}</span>
          </div>
          <div className="flex flex-col gap-[10px]">
            <div className="flex flex-col gap-[5px]">
              <FieldLabel>UUID</FieldLabel>
              <CopyField value={team.id} label="UUID" />
            </div>
            <div className="flex flex-col gap-[5px]">
              <FieldLabel>Slug</FieldLabel>
              <CopyField value={team.slug} label="Slug" />
            </div>
          </div>
        </Card>
      ))}
    </div>
  )
}
