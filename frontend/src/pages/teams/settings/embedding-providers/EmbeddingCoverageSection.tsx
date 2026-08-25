import { AlertCircle, Loader2, RefreshCw, Trash2 } from 'lucide-react'
import type { ReactNode } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type {
  EmbeddingCoverageItem,
  EmbeddingCoverageResponse,
} from '@/services/embeddingProviderService'

const ENTITY_TYPE_LABELS: Record<EmbeddingCoverageItem['entity_type'], string> =
  {
    prompt: 'Prompts',
    artifact: 'Artifacts',
    memory: 'Memories',
    blueprint: 'Blueprints',
    feed_item: 'Feed items',
  }

function entityTypeLabel(type: EmbeddingCoverageItem['entity_type']) {
  return ENTITY_TYPE_LABELS[type]
}

// Percentage guarded against a zero denominator so N=0 renders 0%, never NaN.
function percent(embedded: number, total: number) {
  if (total <= 0) return 0
  return Math.round((embedded / total) * 100)
}
interface StatCardProps {
  label: string
  value: string
  hint?: string
}

function StatCard({ label, value, hint }: Readonly<StatCardProps>) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground text-sm font-medium">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold">{value}</p>
        {hint && <p className="text-muted-foreground mt-1 text-xs">{hint}</p>}
      </CardContent>
    </Card>
  )
}

function EmbeddingCoverageCards({
  coverage,
}: Readonly<{
  coverage: EmbeddingCoverageResponse
}>) {
  const totals = coverage.coverage.reduce(
    (acc, item) => ({
      total: acc.total + item.total,
      embedded: acc.embedded + item.embedded,
      pending: acc.pending + item.pending,
    }),
    { total: 0, embedded: 0, pending: 0 }
  )
  const overallPercent = percent(totals.embedded, totals.total)

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
        <StatCard
          label="Embedded"
          value={totals.embedded.toLocaleString()}
          hint={`of ${totals.total.toLocaleString()} items`}
        />
        <StatCard
          label="Pending"
          value={totals.pending.toLocaleString()}
          hint="waiting for an embedding"
        />
        <StatCard label="% embedded" value={`${String(overallPercent)}%`} />
      </div>

      <div>
        <h3 className="text-muted-foreground mb-2 text-sm font-medium">
          By type
        </h3>
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-5">
          {coverage.coverage.map(item => (
            <StatCard
              key={item.entity_type}
              label={entityTypeLabel(item.entity_type)}
              value={`${String(item.embedded_percent)}%`}
              hint={`${item.embedded.toLocaleString()} / ${item.total.toLocaleString()} · ${item.pending.toLocaleString()} pending`}
            />
          ))}
        </div>
      </div>

      <p className="text-muted-foreground text-xs">
        Pending is the number of items still waiting for an embedding. If it
        isn&rsquo;t going down over time, embedding may be stuck &mdash; use
        &ldquo;Reprocess pending&rdquo; to re-drive it.
      </p>
    </div>
  )
}

interface CoverageSectionProps {
  coverage: EmbeddingCoverageResponse | null
  coverageLoading: boolean
  coverageError: string | null
  canReprocess: boolean
  reprocessing: boolean
  onReprocess: () => void
  canClear: boolean
  clearing: boolean
  onClear: () => void
}

// Coverage summary plus its two maintenance actions (reprocess missing, clear
// all). Extracted from EmbeddingProviders so the page component stays within the
// max-lines-per-function budget.
export function CoverageSection({
  coverage,
  coverageLoading,
  coverageError,
  canReprocess,
  reprocessing,
  onReprocess,
  canClear,
  clearing,
  onClear,
}: Readonly<CoverageSectionProps>) {
  // Loading / error pre-empt the cards; null when there is no coverage yet.
  let coverageContent: ReactNode = null
  if (coverageLoading) {
    coverageContent = (
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-24 w-full" />
        ))}
      </div>
    )
  } else if (coverageError) {
    coverageContent = (
      <Alert variant="destructive">
        <AlertCircle className="size-4" />
        <AlertTitle>Couldn&rsquo;t load embedding coverage</AlertTitle>
        <AlertDescription>{coverageError}</AlertDescription>
      </Alert>
    )
  } else if (coverage) {
    coverageContent = <EmbeddingCoverageCards coverage={coverage} />
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">Embedding coverage</h2>
          <p className="text-muted-foreground text-sm">
            {coverage?.has_active_provider && coverage.active_model
              ? `Measured against ${coverage.active_model}.`
              : 'Embedding status across your content.'}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={!canReprocess || reprocessing}
            onClick={onReprocess}
          >
            {reprocessing ? (
              <Loader2 className="mr-2 size-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 size-4" />
            )}
            Reprocess pending
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="text-destructive hover:text-destructive"
            disabled={!canClear || clearing}
            onClick={onClear}
          >
            {clearing ? (
              <Loader2 className="mr-2 size-4 animate-spin" />
            ) : (
              <Trash2 className="mr-2 size-4" />
            )}
            Clear all embeddings
          </Button>
        </div>
      </div>

      {coverageContent}
    </div>
  )
}
