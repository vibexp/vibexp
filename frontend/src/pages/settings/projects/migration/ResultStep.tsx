import { CheckCircle, XCircle } from 'lucide-react'
import { Link } from 'react-router-dom'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import type {
  MigrationResult,
  ResourceMigrationOutcomes,
} from '@/services/projectMigrationService'

interface CountRowProps {
  label: string
  value: number
}

function CountRow({ label, value }: CountRowProps) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value}</span>
    </div>
  )
}

function countOutcomes(outcomes: ResourceMigrationOutcomes): number {
  return (
    (outcomes.prompts?.length ?? 0) +
    (outcomes.artifacts?.length ?? 0) +
    (outcomes.blueprints?.length ?? 0) +
    (outcomes.feed_items?.length ?? 0)
  )
}

interface OutcomeSectionProps {
  label: string
  colorClass: string
  count: number
  outcomes: ResourceMigrationOutcomes
}

function OutcomeSection({
  label,
  colorClass,
  count,
  outcomes,
}: OutcomeSectionProps) {
  if (count === 0) return null
  return (
    <>
      <Separator />
      <div>
        <p className={`mb-2 text-sm font-medium ${colorClass}`}>
          {label} ({count})
        </p>
        <div className="space-y-1">
          <CountRow label="Prompts" value={outcomes.prompts?.length ?? 0} />
          <CountRow label="Artifacts" value={outcomes.artifacts?.length ?? 0} />
          <CountRow
            label="Blueprints"
            value={outcomes.blueprints?.length ?? 0}
          />
          <CountRow
            label="Feed items"
            value={outcomes.feed_items?.length ?? 0}
          />
        </div>
      </div>
    </>
  )
}

interface ResultStepProps {
  result: MigrationResult
  destinationProjectSlug: string
  destinationProjectName: string
  onDone: () => void
}

export function ResultStep({
  result,
  destinationProjectSlug,
  destinationProjectName,
  onDone,
}: ResultStepProps) {
  const totalMigrated =
    (result.migrated.prompts ?? 0) +
    (result.migrated.artifacts ?? 0) +
    (result.migrated.blueprints ?? 0) +
    (result.migrated.feed_items ?? 0)

  const skippedCount = countOutcomes(result.skipped)
  const failedCount = countOutcomes(result.failed)
  const hasFailures = failedCount > 0

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Migration complete</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          Resources have been moved to{' '}
          <span className="font-medium">{destinationProjectName}</span>.
        </p>
      </div>

      {hasFailures ? (
        <Alert variant="destructive">
          <XCircle className="size-4" />
          <AlertTitle>Some resources failed to migrate</AlertTitle>
          <AlertDescription>
            {failedCount} resource{failedCount === 1 ? '' : 's'} could not be
            moved. Review the counts below.
          </AlertDescription>
        </Alert>
      ) : (
        <Alert>
          <CheckCircle className="size-4" />
          <AlertTitle>Migration successful</AlertTitle>
          <AlertDescription>
            {totalMigrated} resource{totalMigrated === 1 ? '' : 's'} moved
            successfully.
          </AlertDescription>
        </Alert>
      )}

      <div className="rounded-lg border p-4 space-y-4">
        <div>
          <p className="mb-2 text-sm font-medium">Migrated</p>
          <div className="space-y-1">
            <CountRow label="Prompts" value={result.migrated.prompts ?? 0} />
            <CountRow
              label="Artifacts"
              value={result.migrated.artifacts ?? 0}
            />
            <CountRow
              label="Blueprints"
              value={result.migrated.blueprints ?? 0}
            />
            <CountRow
              label="Feed items"
              value={result.migrated.feed_items ?? 0}
            />
          </div>
        </div>

        <OutcomeSection
          label="Skipped"
          colorClass="text-warning"
          count={skippedCount}
          outcomes={result.skipped}
        />

        <OutcomeSection
          label="Failed"
          colorClass="text-destructive"
          count={failedCount}
          outcomes={result.failed}
        />
      </div>

      <div className="flex flex-wrap gap-2">
        <Button asChild>
          <Link
            to={`/settings/projects/${encodeURIComponent(destinationProjectSlug)}`}
          >
            View destination project
          </Link>
        </Button>
        <Button variant="outline" onClick={onDone}>
          Done
        </Button>
      </div>
    </div>
  )
}
