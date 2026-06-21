import {
  ArrowLeft,
  ArrowLeftRight,
  ChevronDown,
  Columns2,
  EllipsisVertical,
  GitCompare,
  RotateCcw,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import { Diffstat, VersionAuthorChip } from './atoms'
import { computeDiffStat } from './diff'
import { formatRelative, formatTimestamp } from './format'
import type { VersionTimelineEntry } from './types'
import { type DiffMode, VersionDiff } from './VersionDiff'

interface VersionCompareViewProps {
  entries: VersionTimelineEntry[]
  baseVersion: number
  compareVersion: number
  onChangeBase: (versionNumber: number) => void
  onChangeCompare: (versionNumber: number) => void
  onSwap: () => void
  onClose: () => void
  onRestore: (versionNumber: number) => void
}

function CurrentTag() {
  return (
    <span className="vhb-current-tag rounded-md bg-secondary px-1.5 py-0.5 text-secondary-foreground">
      Current
    </span>
  )
}

function VersionChip({
  side,
  entry,
  otherVersion,
  entries,
  onPick,
}: {
  side: 'base' | 'compare'
  entry: VersionTimelineEntry
  otherVersion: number
  entries: VersionTimelineEntry[]
  onPick: (versionNumber: number) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="vcmp-chip"
          data-testid={`compare-chip-${side}`}
        >
          <span
            className="vcmp-chip__sw"
            style={{
              background:
                side === 'base' ? 'var(--muted-foreground)' : 'var(--primary)',
            }}
          />
          <span className="vcmp-chip__main">
            <span className="vcmp-chip__v">
              Version {entry.versionNumber}
              {entry.isCurrent && <CurrentTag />}
            </span>
            <span className="vcmp-chip__t">
              {formatTimestamp(entry.createdAt)}
            </span>
          </span>
          <span className="vcmp-chip__chev" aria-hidden="true">
            <ChevronDown />
          </span>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={side === 'base' ? 'start' : 'end'}>
        {entries.map(e => (
          <DropdownMenuItem
            key={e.versionNumber}
            disabled={e.versionNumber === otherVersion}
            onSelect={() => {
              onPick(e.versionNumber)
            }}
          >
            Version {e.versionNumber}
            {e.isCurrent ? ' · current' : ''}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// Full-page compare takeover (design `CompareTakeover`). The two chips ARE the
// comparison — re-pick either side or swap. Diff is base (old) → compare (new).
export function VersionCompareView({
  entries,
  baseVersion,
  compareVersion,
  onChangeBase,
  onChangeCompare,
  onSwap,
  onClose,
  onRestore,
}: VersionCompareViewProps) {
  const [mode, setMode] = useState<DiffMode>('split')

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [onClose])

  const base = entries.find(e => e.versionNumber === baseVersion)
  const compare = entries.find(e => e.versionNumber === compareVersion)

  const stat = useMemo(
    () =>
      base && compare ? computeDiffStat(base.content, compare.content) : null,
    [base, compare]
  )

  if (!base || !compare) return null

  return (
    <div className="vh-compare" data-testid="version-compare-view">
      <div className="vcmp-bar">
        <Button
          variant="outline"
          size="sm"
          className="vcmp-back"
          onClick={onClose}
        >
          <ArrowLeft />
          Back to history
        </Button>

        <div className="vcmp-chips">
          <VersionChip
            side="base"
            entry={base}
            otherVersion={compareVersion}
            entries={entries}
            onPick={onChangeBase}
          />
          <Button
            variant="ghost"
            size="icon"
            className="vcmp-swap"
            title="Swap sides"
            aria-label="Swap base and compare"
            onClick={onSwap}
          >
            <ArrowLeftRight />
          </Button>
          <VersionChip
            side="compare"
            entry={compare}
            otherVersion={baseVersion}
            entries={entries}
            onPick={onChangeCompare}
          />
        </div>

        <div className="vcmp-bar__actions">
          <div className="vcmp-seg" role="group" aria-label="Diff layout">
            <button
              type="button"
              className={cn(mode === 'unified' && 'is-on')}
              aria-pressed={mode === 'unified'}
              onClick={() => {
                setMode('unified')
              }}
            >
              <GitCompare />
              Unified
            </button>
            <button
              type="button"
              className={cn(mode === 'split' && 'is-on')}
              aria-pressed={mode === 'split'}
              onClick={() => {
                setMode('split')
              }}
            >
              <Columns2 />
              Split
            </button>
          </div>
          {base.restorable && (
            <Button
              variant="outline"
              size="sm"
              data-testid="compare-restore-button"
              onClick={() => {
                onRestore(base.versionNumber)
              }}
            >
              <RotateCcw />
              Restore v{base.versionNumber}
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label="More compare actions"
                style={{ width: '2.25rem', height: '2.25rem' }}
              >
                <EllipsisVertical />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={onSwap}>Swap sides</DropdownMenuItem>
              {base.restorable && (
                <DropdownMenuItem
                  onSelect={() => {
                    onRestore(base.versionNumber)
                  }}
                >
                  Restore v{base.versionNumber}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem onSelect={onClose}>
                Back to history
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <div className="vcmp-meta">
        <div className="vcmp-mcol">
          <span className="vcmp-mtag vcmp-mtag--base">Base</span>
          <span className="vcmp-minfo">
            <span className="vcmp-mv">Version {base.versionNumber}</span>
            <span className="vcmp-msub">
              <VersionAuthorChip entry={base} small /> ·{' '}
              {formatRelative(base.createdAt)}
            </span>
          </span>
        </div>
        <div className="vcmp-mcol">
          <span className="vcmp-mtag vcmp-mtag--cmp">Compare</span>
          <span className="vcmp-minfo">
            <span className="vcmp-mv">
              Version {compare.versionNumber}
              {compare.isCurrent && <CurrentTag />}
            </span>
            <span className="vcmp-msub">
              <VersionAuthorChip entry={compare} small /> ·{' '}
              {formatRelative(compare.createdAt)}
            </span>
          </span>
          <span className="vcmp-mright">
            <Diffstat stat={stat} />
          </span>
        </div>
      </div>

      <div className="vcmp-diffwrap">
        <VersionDiff
          mode={mode}
          oldText={base.content}
          newText={compare.content}
          oldLabel={`Version ${String(base.versionNumber)}`}
          newLabel={`Version ${String(compare.versionNumber)}${compare.isCurrent ? ' · current' : ''}`}
        />
      </div>
    </div>
  )
}
