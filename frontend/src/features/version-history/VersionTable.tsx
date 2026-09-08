import { ChevronDown, History } from 'lucide-react'

import { TABLE_HEAD_CLASS } from '@/components/patterns/list-page'
import { cn } from '@/lib/utils'

import type { VersionTimelineEntry } from './types'
import { VersionRow } from './VersionRow'

export type SortKey = 'version' | 'when'
export type SortDir = 'asc' | 'desc'

interface VersionTableProps {
  entries: VersionTimelineEntry[]
  hasSnapshots: boolean
  selected: number[]
  onToggleSort: (key: SortKey) => void
  onToggleSelect: (versionNumber: number) => void
  onView: (entry: VersionTimelineEntry) => void
  onRestore: (versionNumber: number) => void
}

export function VersionTable({
  entries,
  hasSnapshots,
  selected,
  onToggleSort,
  onToggleSelect,
  onView,
  onRestore,
}: Readonly<VersionTableProps>) {
  if (!hasSnapshots) {
    return (
      <div className="vhc-tablewrap">
        <div className="vhc-empty">
          <History aria-hidden="true" />
          <p>No previous versions yet.</p>
          <p className="text-sm">
            A version is recorded each time the content changes.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="vhc-tablewrap">
      <table className="vhc-table">
        <thead>
          {/* Every head cell carries TABLE_HEAD_CLASS so this table's header
              reads exactly like a `ListTable`'s (#909). */}
          <tr>
            <th className={TABLE_HEAD_CLASS} style={{ width: 40 }} />
            <th
              className={cn(TABLE_HEAD_CLASS, 'vhc-sort')}
              style={{ width: 150 }}
              onClick={() => {
                onToggleSort('version')
              }}
            >
              {'Version'}
              <span className="vhc-sortic" aria-hidden="true">
                <ChevronDown />
              </span>
            </th>
            <th className={TABLE_HEAD_CLASS}>Change summary</th>
            <th className={TABLE_HEAD_CLASS} style={{ width: 150 }}>
              Changed by
            </th>
            <th
              className={cn(TABLE_HEAD_CLASS, 'vhc-sort')}
              style={{ width: 150 }}
              onClick={() => {
                onToggleSort('when')
              }}
            >
              {'When'}
              <span className="vhc-sortic" aria-hidden="true">
                <ChevronDown />
              </span>
            </th>
            <th className={TABLE_HEAD_CLASS} style={{ width: 130 }}>
              Changes
            </th>
            <th className={TABLE_HEAD_CLASS} style={{ width: 120 }} />
          </tr>
        </thead>
        <tbody>
          {entries.map(entry => (
            <VersionRow
              key={entry.versionNumber}
              entry={entry}
              selected={selected.includes(entry.versionNumber)}
              onToggleSelect={onToggleSelect}
              onView={onView}
              onRestore={onRestore}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}
