import { Check, Columns2, EllipsisVertical, Eye, RotateCcw } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import { Diffstat, VersionAuthorChip, VersionLabel } from './atoms'
import { formatTimestamp } from './format'
import type { VersionTimelineEntry } from './types'

interface VersionRowProps {
  entry: VersionTimelineEntry
  selected: boolean
  onToggleSelect: (versionNumber: number) => void
  onView: (entry: VersionTimelineEntry) => void
  onRestore: (versionNumber: number) => void
}

export function VersionRow({
  entry,
  selected,
  onToggleSelect,
  onView,
  onRestore,
}: VersionRowProps) {
  const label = String(entry.versionNumber)
  return (
    <tr
      className={cn(selected && 'is-sel')}
      onClick={() => {
        onToggleSelect(entry.versionNumber)
      }}
    >
      <td>
        <button
          type="button"
          className={cn('vhc-check', selected && 'is-on')}
          role="checkbox"
          aria-checked={selected}
          aria-label={`Select version ${label}`}
          onClick={e => {
            e.stopPropagation()
            onToggleSelect(entry.versionNumber)
          }}
        >
          {selected && <Check />}
        </button>
      </td>
      <td>
        <VersionLabel entry={entry} withDot />
      </td>
      <td className="vhc-summary">{entry.changeSummary ?? '—'}</td>
      <td>
        <VersionAuthorChip entry={entry} small />
      </td>
      <td className="vhc-when">{formatTimestamp(entry.createdAt)}</td>
      <td>
        <Diffstat stat={entry.stat} />
      </td>
      <td className="vhc-actcell">
        <span className="vhc-actbtns">
          <Button
            variant="ghost"
            size="icon"
            className="vhc-rowbtn"
            title="View"
            aria-label={`Compare version ${label}`}
            onClick={e => {
              e.stopPropagation()
              onView(entry)
            }}
          >
            <Eye />
          </Button>
          {entry.restorable && (
            <Button
              variant="ghost"
              size="icon"
              className="vhc-rowbtn"
              title="Restore"
              aria-label={`Restore version ${label}`}
              onClick={e => {
                e.stopPropagation()
                onRestore(entry.versionNumber)
              }}
            >
              <RotateCcw />
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="vhc-rowbtn"
                title="More"
                aria-label={`More actions for version ${label}`}
                onClick={e => {
                  e.stopPropagation()
                }}
              >
                <EllipsisVertical />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="end"
              onClick={e => {
                e.stopPropagation()
              }}
            >
              <DropdownMenuItem
                onSelect={() => {
                  onView(entry)
                }}
              >
                <Columns2 className="size-4" />
                Compare with current
              </DropdownMenuItem>
              {entry.restorable && (
                <DropdownMenuItem
                  onSelect={() => {
                    onRestore(entry.versionNumber)
                  }}
                >
                  <RotateCcw className="size-4" />
                  Restore this version
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </span>
      </td>
    </tr>
  )
}
