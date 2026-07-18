import './version-history.css'

import { ArrowLeft, CircleAlert } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useAlerts } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { getErrorMessage } from '@/utils/errorHandling'

import { RestoreVersionDialog } from './RestoreVersionDialog'
import { buildTimeline } from './timeline'
import type {
  VersionHistorySource,
  VersionTimelineData,
  VersionTimelineEntry,
} from './types'
import { VersionCompareView } from './VersionCompareView'
import { type SortDir, type SortKey, VersionTable } from './VersionTable'
import { type DateRange, VersionToolbar } from './VersionToolbar'

const RANGE_MS: Record<Exclude<DateRange, 'all'>, number> = {
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
  '30d': 30 * 24 * 60 * 60 * 1000,
}

interface VersionHistoryPageProps {
  source: VersionHistorySource
}

// Resource-agnostic version-history experience: dense filterable list + full-page
// compare + non-destructive restore. Configured for a resource via `source`.
export function VersionHistoryPage({
  source,
}: Readonly<VersionHistoryPageProps>) {
  const navigate = useNavigate()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()

  const [data, setData] = useState<VersionTimelineData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [search, setSearch] = useState('')
  const [authorFilter, setAuthorFilter] = useState<string | null>(null)
  const [dateRange, setDateRange] = useState<DateRange>('all')
  const [sortKey, setSortKey] = useState<SortKey>('version')
  const [sortDir, setSortDir] = useState<SortDir>('desc')

  const [selected, setSelected] = useState<number[]>([])
  const [compareOpen, setCompareOpen] = useState(false)
  const [baseVersion, setBaseVersion] = useState<number | null>(null)
  const [compareVersion, setCompareVersion] = useState<number | null>(null)

  const [restoreTarget, setRestoreTarget] = useState<number | null>(null)
  const [restoring, setRestoring] = useState(false)

  const load = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      setData(await source.load())
    } catch (err) {
      setError(getErrorMessage(err, 'Failed to fetch version history'))
      handleError(err, 'Failed to load version history')
    } finally {
      setLoading(false)
    }
  }, [source, handleError])

  useEffect(() => {
    void load()
  }, [load])

  const timeline = useMemo(() => (data ? buildTimeline(data) : []), [data])
  const snapshotCount = data?.versions.length ?? 0
  const nextVersionNumber = timeline.length > 0 ? timeline[0].versionNumber : 1

  const authors = useMemo(() => {
    const map = new Map<string, string>()
    for (const entry of timeline) {
      if (entry.author) map.set(entry.author.id, entry.author.display_name)
    }
    return [...map.entries()].map(([id, name]) => ({ id, name }))
  }, [timeline])

  const visibleEntries = useMemo(() => {
    const now = Date.now()
    const term = search.trim().toLowerCase()
    const filtered = timeline.filter(entry => {
      if (authorFilter && entry.author?.id !== authorFilter) return false
      if (dateRange !== 'all') {
        const age = now - new Date(entry.createdAt).getTime()
        if (Number.isNaN(age) || age > RANGE_MS[dateRange]) return false
      }
      if (term) {
        const haystack = [
          entry.changeSummary ?? '',
          entry.author?.display_name ?? '',
          `version ${String(entry.versionNumber)}`,
        ]
          .join(' ')
          .toLowerCase()
        if (!haystack.includes(term)) return false
      }
      return true
    })

    return [...filtered].sort((a, b) => {
      const cmp =
        sortKey === 'version'
          ? a.versionNumber - b.versionNumber
          : new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
      return sortDir === 'asc' ? cmp : -cmp
    })
  }, [timeline, authorFilter, dateRange, search, sortKey, sortDir])

  const toggleSort = useCallback(
    (key: SortKey) => {
      if (sortKey === key) {
        setSortDir(prev => (prev === 'asc' ? 'desc' : 'asc'))
      } else {
        setSortKey(key)
        setSortDir('desc')
      }
    },
    [sortKey]
  )

  const toggleSelect = useCallback((versionNumber: number) => {
    setSelected(prev => {
      if (prev.includes(versionNumber))
        return prev.filter(v => v !== versionNumber)
      if (prev.length < 2) return [...prev, versionNumber]
      return [prev[1], versionNumber] // keep latest pick + new, drop oldest
    })
  }, [])

  const openCompare = useCallback((a: number, b: number) => {
    const [older, newer] = a < b ? [a, b] : [b, a]
    setBaseVersion(older)
    setCompareVersion(newer)
    setCompareOpen(true)
  }, [])

  const handleCompareSelected = useCallback(() => {
    if (selected.length === 2) openCompare(selected[0], selected[1])
  }, [selected, openCompare])

  const handleViewEntry = useCallback(
    (entry: VersionTimelineEntry) => {
      if (timeline.length === 0) return
      const current = timeline[0]
      let other: VersionTimelineEntry | undefined
      if (entry.versionNumber === current.versionNumber) {
        other = timeline.length > 1 ? timeline[1] : undefined
      } else {
        other = current
      }
      if (other) openCompare(entry.versionNumber, other.versionNumber)
    },
    [timeline, openCompare]
  )

  const handleRestore = useCallback(async () => {
    if (restoreTarget === null) return
    try {
      setRestoring(true)
      await source.restore(restoreTarget)
      showSuccess('Version restored successfully', 'Success')
      setRestoreTarget(null)
      setCompareOpen(false)
      setSelected([])
      await load()
    } catch (err) {
      handleError(err, 'Failed to restore version')
    } finally {
      setRestoring(false)
    }
  }, [restoreTarget, source, showSuccess, load, handleError])

  if (loading) {
    return (
      <div className="vh-root">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error || !data) {
    return (
      <div className="vh-root space-y-6">
        <Alert variant="destructive">
          <CircleAlert className="size-4" />
          <AlertTitle>Version history unavailable</AlertTitle>
          <AlertDescription>
            {error ?? 'Could not load version history.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => void navigate(source.backHref)}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back
        </Button>
      </div>
    )
  }

  return (
    <div className="vh-root">
      <div className="vh-list">
        <PageHeader
          title="Version history"
          description={`${data.resourceName} · ${String(timeline.length)} version${timeline.length === 1 ? '' : 's'} · review prior snapshots, compare changes, and restore any version.`}
          actions={
            <Button
              variant="outline"
              size="sm"
              data-testid="back-to-resource-button"
              onClick={() => void navigate(source.backHref)}
            >
              <ArrowLeft />
              Back to {source.resourceLabel}
            </Button>
          }
        />

        <VersionToolbar
          search={search}
          onSearchChange={setSearch}
          authors={authors}
          authorFilter={authorFilter}
          onAuthorFilter={setAuthorFilter}
          dateRange={dateRange}
          onDateRange={setDateRange}
          selectedCount={selected.length}
          onCompare={handleCompareSelected}
        />

        <VersionTable
          entries={visibleEntries}
          hasSnapshots={snapshotCount > 0}
          selected={selected}
          onToggleSort={toggleSort}
          onToggleSelect={toggleSelect}
          onView={handleViewEntry}
          onRestore={setRestoreTarget}
        />
      </div>

      {compareOpen && baseVersion !== null && compareVersion !== null && (
        <VersionCompareView
          entries={timeline}
          baseVersion={baseVersion}
          compareVersion={compareVersion}
          onChangeBase={setBaseVersion}
          onChangeCompare={setCompareVersion}
          onSwap={() => {
            setBaseVersion(compareVersion)
            setCompareVersion(baseVersion)
          }}
          onClose={() => {
            setCompareOpen(false)
          }}
          onRestore={setRestoreTarget}
        />
      )}

      <RestoreVersionDialog
        open={restoreTarget !== null}
        onOpenChange={open => {
          if (!open) setRestoreTarget(null)
        }}
        versionNumber={restoreTarget}
        nextVersionNumber={nextVersionNumber}
        loading={restoring}
        onConfirm={() => void handleRestore()}
      />
    </div>
  )
}
