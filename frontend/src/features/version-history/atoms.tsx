import { cn } from '@/lib/utils'

import type { DiffStat } from './diff'
import type { VersionTimelineEntry } from './types'

// Resolved attribution for rendering one timeline entry's author. The three
// avatar tones are the design's single documented raw-colour exception (defined
// in version-history.css): tone 3 (orange) flags a system action; humans get a
// stable tone 1/2 from their id.
interface ResolvedAuthor {
  initials: string
  name: string
  avatarUrl: string | null
  tone: 1 | 2 | 3
}

function toneFromId(id: string): 1 | 2 {
  let hash = 0
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) >>> 0
  }
  return hash % 2 === 0 ? 1 : 2
}

export function resolveAuthor(
  entry: VersionTimelineEntry
): ResolvedAuthor | null {
  if (entry.author) {
    return {
      initials: entry.author.initials,
      name: entry.author.display_name,
      avatarUrl: entry.author.avatar_url,
      tone: entry.actorType === 'system' ? 3 : toneFromId(entry.author.id),
    }
  }
  // A system action with no resolvable user → the auto-save persona.
  if (entry.actorType === 'system') {
    return { initials: '◆', name: 'Auto-save', avatarUrl: null, tone: 3 }
  }
  return null
}

export function VersionAvatar({
  author,
  small,
}: {
  author: ResolvedAuthor
  small?: boolean
}) {
  return (
    <span
      className={cn(
        'vhp-ava',
        `vhp-ava--${String(author.tone)}`,
        small && 'vhp-ava--s'
      )}
      aria-hidden="true"
    >
      {author.avatarUrl ? (
        <img src={author.avatarUrl} alt="" />
      ) : (
        author.initials
      )}
    </span>
  )
}

// Avatar + name. Falls back to an em-dash when there is no author (e.g. the live
// draft, which has no committed snapshot yet).
export function VersionAuthorChip({
  entry,
  small,
}: {
  entry: VersionTimelineEntry
  small?: boolean
}) {
  const author = resolveAuthor(entry)
  if (!author) {
    return <span className="vhp-stat__none">—</span>
  }
  return (
    <span className="vhp-who">
      <VersionAvatar author={author} small={small} />
      <span className="vhp-name">{author.name}</span>
    </span>
  )
}

// +N −M diffstat with up to five proportional squares. Em-dash when there is no
// prior version to diff against.
export function Diffstat({ stat }: { stat: DiffStat | null }) {
  if (!stat || (stat.added === 0 && stat.removed === 0)) {
    return <span className="vhp-stat__none">—</span>
  }
  const { added, removed } = stat
  const total = Math.min(added + removed, 5)
  const nAdd = Math.round((added / (added + removed)) * total)
  const squares = Array.from({ length: 5 }, (_, i) =>
    i < nAdd ? 'is-add' : i < total ? 'is-del' : ''
  )
  return (
    <span className="vhp-stat">
      <span className="vhp-stat__add">+{added}</span>
      <span className="vhp-stat__del">−{removed}</span>
      <span className="vhp-stat__bars">
        {squares.map((c, i) => (
          <i key={i} className={c} />
        ))}
      </span>
    </span>
  )
}

// Render a version label, optionally with the neutral "Current" tag.
export function VersionLabel({
  entry,
  withDot,
}: {
  entry: VersionTimelineEntry
  withDot?: boolean
}) {
  return (
    <span className="vhc-vcell">
      {withDot && (
        <span
          className={cn('vhc-vdot', entry.isCurrent && 'is-cur')}
          aria-hidden="true"
        />
      )}
      Version {entry.versionNumber}
      {entry.isCurrent && (
        <span className="vhb-current-tag rounded-md bg-secondary px-1.5 py-0.5 text-secondary-foreground">
          Current
        </span>
      )}
    </span>
  )
}
