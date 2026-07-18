import { useMemo } from 'react'

import { cn } from '@/lib/utils'

import {
  buildSplitRows,
  buildUnifiedRows,
  hunkHeader,
  type SplitRow,
  type WordSegment,
} from './diff'

export type DiffMode = 'split' | 'unified'

// Zero-width space: keeps a blank line's row at full line-height without adding
// any visible/selectable content.
const ZWSP = String.fromCodePoint(0x200b)

interface VersionDiffProps {
  oldText: string
  newText: string
  oldLabel: string
  newLabel: string
  mode: DiffMode
}

// Render a line's word segments, highlighting the changed runs. A blank line is
// rendered as a zero-width space so its grid row still has line-height.
function LineSegments({ segments }: Readonly<{ segments: WordSegment[] }>) {
  if (segments.every(s => s.text.length === 0)) {
    return <>{ZWSP}</>
  }
  // Key each run by its character offset within the line: segments are a linear
  // decomposition of the line (diffWords runs are non-empty), so offsets are
  // strictly increasing and unique.
  let offset = 0
  return (
    <>
      {segments.map(segment => {
        const key = `${String(offset)}-${segment.changed ? 'c' : 'u'}`
        offset += segment.text.length
        return segment.changed ? (
          <span key={key} className="vcmp-word">
            {segment.text}
          </span>
        ) : (
          <span key={key}>{segment.text}</span>
        )
      })}
    </>
  )
}

function leftClass(row: SplitRow): string {
  if (row.left.segments === null) return 'is-empty'
  return row.kind === 'del' || row.kind === 'mod' ? 'is-del' : ''
}

function rightClass(row: SplitRow): string {
  if (row.right.segments === null) return 'is-empty'
  return row.kind === 'add' || row.kind === 'mod' ? 'is-add' : ''
}

function SplitDiff({
  oldText,
  newText,
  oldLabel,
  newLabel,
}: Readonly<Omit<VersionDiffProps, 'mode'>>) {
  const rows = useMemo(
    () => buildSplitRows(oldText, newText),
    [oldText, newText]
  )
  const hunk = useMemo(() => hunkHeader(oldText, newText), [oldText, newText])

  return (
    <div className="vcmp-split" data-testid="version-diff-split">
      <div className="vcmp-colhead">
        <div>{oldLabel}</div>
        <div>{newLabel}</div>
      </div>
      <div className="vcmp-hunk">{hunk}</div>
      {/* Every row consumes at least one fresh line number on one side, so the
          left/right number pair is unique per row. */}
      {rows.map(row => (
        <div
          className="vcmp-srow"
          key={`${String(row.left.num ?? 'x')}-${String(row.right.num ?? 'x')}`}
        >
          <span className="vcmp-num">{row.left.num ?? ''}</span>
          <span className={cn('vcmp-code', leftClass(row))}>
            {row.left.segments && <LineSegments segments={row.left.segments} />}
          </span>
          <span className="vcmp-num vcmp-numr">{row.right.num ?? ''}</span>
          <span className={cn('vcmp-code', rightClass(row))}>
            {row.right.segments && (
              <LineSegments segments={row.right.segments} />
            )}
          </span>
        </div>
      ))}
    </div>
  )
}

const UNIFIED_MARK: Record<string, string> = {
  add: '+',
  del: '−',
  context: ' ',
}

function UnifiedDiff({
  oldText,
  newText,
}: Readonly<Omit<VersionDiffProps, 'mode' | 'oldLabel' | 'newLabel'>>) {
  const rows = useMemo(
    () => buildUnifiedRows(oldText, newText),
    [oldText, newText]
  )
  const hunk = useMemo(() => hunkHeader(oldText, newText), [oldText, newText])

  return (
    <div className="vha-diff" data-testid="version-diff-unified">
      <div className="vha-hunk">{hunk}</div>
      {/* Every row consumes at least one fresh line number on one side, so the
          left/right number pair is unique per row. */}
      {rows.map(row => (
        <div
          className={cn(
            'vha-dline',
            row.kind === 'add' && 'is-add',
            row.kind === 'del' && 'is-del'
          )}
          key={`${String(row.leftNum ?? 'x')}-${String(row.rightNum ?? 'x')}`}
        >
          <span className="vha-dnum">{row.leftNum ?? ''}</span>
          <span className="vha-dnum">{row.rightNum ?? ''}</span>
          <span className="vha-dtext">
            <span className="vha-dmark" aria-hidden="true">
              {UNIFIED_MARK[row.kind]}
            </span>
            {row.text === '' ? ZWSP : row.text}
          </span>
        </div>
      ))}
    </div>
  )
}

export function VersionDiff({
  oldText,
  newText,
  oldLabel,
  newLabel,
  mode,
}: Readonly<VersionDiffProps>) {
  if (mode === 'unified') {
    return <UnifiedDiff oldText={oldText} newText={newText} />
  }
  return (
    <SplitDiff
      oldText={oldText}
      newText={newText}
      oldLabel={oldLabel}
      newLabel={newLabel}
    />
  )
}
