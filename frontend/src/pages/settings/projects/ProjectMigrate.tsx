import { AlertCircle, ArrowLeft } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { ResultStep } from '@/pages/settings/projects/migration/ResultStep'
import { SelectDestinationStep } from '@/pages/settings/projects/migration/SelectDestinationStep'
import { SelectResourcesStep } from '@/pages/settings/projects/migration/SelectResourcesStep'
import { SelectSourceStep } from '@/pages/settings/projects/migration/SelectSourceStep'
import {
  type ConflictPolicy,
  type MigrationInventory,
  type MigrationResult,
  projectMigrationService,
  type ResourceSelections,
} from '@/services/projectMigrationService'
import { type Project, projectService } from '@/services/projectService'
import { getErrorMessage } from '@/utils/errorHandling'

const EMPTY_SELECTION: ResourceSelections = {
  prompts: { all: false, ids: [] },
  artifacts: { all: false, ids: [] },
  blueprints: { all: false, ids: [] },
  feed_items: { all: false, ids: [] },
}

type WizardStep = 1 | 2 | 3 | 4

interface WizardState {
  step: WizardStep
  sourceProjectId: string
  sourceProjectSlug: string
  sourceProjectName: string
  inventory: MigrationInventory | null
  selectedResources: ResourceSelections
  destinationProjectId: string
  destinationProjectSlug: string
  destinationProjectName: string
  conflictPolicy: ConflictPolicy
  result: MigrationResult | null
  loadingInventory: boolean
  migrating: boolean
  error: string | null
}

const STEP_LABELS: Record<WizardStep, string> = {
  1: 'Source',
  2: 'Resources',
  3: 'Destination',
  4: 'Result',
}

function StepIndicator({ currentStep }: { currentStep: WizardStep }) {
  const steps: WizardStep[] = [1, 2, 3, 4]
  return (
    <div className="flex items-center gap-2" aria-label="Wizard steps">
      {steps.map((step, idx) => (
        <div key={step} className="flex items-center gap-2">
          <div className="flex items-center gap-1.5">
            <div
              className={`flex size-6 items-center justify-center rounded-full text-xs font-medium ${
                step === currentStep
                  ? 'bg-primary text-primary-foreground'
                  : step < currentStep
                    ? 'bg-primary/20 text-primary'
                    : 'bg-muted text-muted-foreground'
              }`}
              aria-current={step === currentStep ? 'step' : undefined}
            >
              {step}
            </div>
            <span
              className={`text-sm ${
                step === currentStep ? 'font-medium' : 'text-muted-foreground'
              }`}
            >
              {STEP_LABELS[step]}
            </span>
          </div>
          {idx < steps.length - 1 && (
            <Separator orientation="horizontal" className="w-8" />
          )}
        </div>
      ))}
    </div>
  )
}

export function ProjectMigrate() {
  const { slug } = useParams<{ slug: string }>()
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()

  const [resolvedSourceProject, setResolvedSourceProject] =
    useState<Project | null>(null)
  const [loadingProjects, setLoadingProjects] = useState(true)
  const [pageError, setPageError] = useState<string | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)

  const [wizard, setWizard] = useState<WizardState>({
    step: 1,
    sourceProjectId: '',
    sourceProjectSlug: slug ?? '',
    sourceProjectName: '',
    inventory: null,
    selectedResources: EMPTY_SELECTION,
    destinationProjectId: '',
    destinationProjectSlug: '',
    destinationProjectName: '',
    conflictPolicy: 'skip',
    result: null,
    loadingInventory: false,
    migrating: false,
    error: null,
  })

  // Resolve the source project from the URL slug. The step pickers fetch their
  // own (server-searched) project lists, so we only need the source here.
  const loadProjects = useCallback(async () => {
    if (!currentTeam || !slug) return
    try {
      setLoadingProjects(true)
      const sourceProject = await projectService.getProject(
        currentTeam.id,
        slug
      )
      setResolvedSourceProject(sourceProject)
      setWizard(prev => ({
        ...prev,
        sourceProjectId: sourceProject.id,
        sourceProjectSlug: sourceProject.slug,
        sourceProjectName: sourceProject.name,
      }))
    } catch (err) {
      const msg = getErrorMessage(err, 'Failed to load projects')
      setPageError(msg)
      handleError(err, 'Failed to load projects')
    } finally {
      setLoadingProjects(false)
    }
  }, [currentTeam, slug, handleError])

  useEffect(() => {
    void loadProjects()
  }, [loadProjects])

  // Step 1 → 2: load inventory when source confirmed
  const handleSourceNext = useCallback(async () => {
    if (!currentTeam || !wizard.sourceProjectId) return
    setWizard(prev => ({ ...prev, loadingInventory: true, error: null }))
    try {
      const inventory = await projectMigrationService.getInventory(
        currentTeam.id,
        wizard.sourceProjectId
      )
      setWizard(prev => ({
        ...prev,
        inventory,
        loadingInventory: false,
        step: 2,
        // Reset selections when source changes
        selectedResources: EMPTY_SELECTION,
      }))
    } catch (err) {
      const msg = getErrorMessage(err, 'Failed to load inventory')
      setWizard(prev => ({
        ...prev,
        loadingInventory: false,
        error: msg,
      }))
      handleError(err, 'Failed to load migration inventory')
    }
  }, [currentTeam, wizard.sourceProjectId, handleError])

  const handleSourceSelect = useCallback((project: Project) => {
    setWizard(prev => ({
      ...prev,
      sourceProjectId: project.id,
      sourceProjectSlug: project.slug,
      sourceProjectName: project.name,
    }))
  }, [])

  const handleResourcesChange = useCallback((resources: ResourceSelections) => {
    setWizard(prev => ({ ...prev, selectedResources: resources }))
  }, [])

  const handleDestinationSelect = useCallback((project: Project) => {
    setWizard(prev => ({
      ...prev,
      destinationProjectId: project.id,
      destinationProjectSlug: project.slug,
      destinationProjectName: project.name,
    }))
  }, [])

  const handleConflictPolicyChange = useCallback((policy: ConflictPolicy) => {
    setWizard(prev => ({ ...prev, conflictPolicy: policy }))
  }, [])

  // Called when user clicks Migrate → opens confirm dialog
  const handleMigrateClick = useCallback(() => {
    setConfirmOpen(true)
  }, [])

  // Called when user confirms in dialog
  const handleConfirmMigrate = useCallback(async () => {
    if (!currentTeam || !wizard.sourceProjectId || !wizard.destinationProjectId)
      return
    setConfirmOpen(false)
    setWizard(prev => ({ ...prev, migrating: true, error: null }))
    try {
      const result = await projectMigrationService.migrate(
        currentTeam.id,
        wizard.sourceProjectId,
        {
          destination_project_id: wizard.destinationProjectId,
          resources: wizard.selectedResources,
          conflict_policy: wizard.conflictPolicy,
        }
      )
      showSuccess('Resources migrated successfully', 'Migration complete')
      setWizard(prev => ({
        ...prev,
        migrating: false,
        result,
        step: 4,
      }))
    } catch (err) {
      const msg = getErrorMessage(err, 'Migration failed')
      setWizard(prev => ({
        ...prev,
        migrating: false,
        error: msg,
      }))
      handleError(err, 'Migration failed')
    }
  }, [
    currentTeam,
    wizard.sourceProjectId,
    wizard.destinationProjectId,
    wizard.selectedResources,
    wizard.conflictPolicy,
    showSuccess,
    handleError,
  ])

  const handleDone = useCallback(() => {
    void navigate('/settings/projects')
  }, [navigate])

  const inventoryCounts = wizard.inventory
    ? {
        prompts: wizard.inventory.prompts?.count ?? 0,
        artifacts: wizard.inventory.artifacts?.count ?? 0,
        blueprints: wizard.inventory.blueprints?.count ?? 0,
        feed_items: wizard.inventory.feed_items?.count ?? 0,
      }
    : { prompts: 0, artifacts: 0, blueprints: 0, feed_items: 0 }

  const confirmDescription =
    wizard.conflictPolicy === 'overwrite' ? (
      <>
        This will move the selected resources out of{' '}
        <span className="font-medium">{wizard.sourceProjectName}</span> into{' '}
        <span className="font-medium">{wizard.destinationProjectName}</span>.
        They will no longer appear in {wizard.sourceProjectName}.{' '}
        <span className="font-semibold text-destructive">
          Existing resources in the destination with matching names will be
          permanently replaced.
        </span>
      </>
    ) : (
      <>
        This will move the selected resources out of{' '}
        <span className="font-medium">{wizard.sourceProjectName}</span> into{' '}
        <span className="font-medium">{wizard.destinationProjectName}</span>.
        They will no longer appear in {wizard.sourceProjectName}.
      </>
    )

  if (loadingProjects) {
    return (
      <div className="mx-auto max-w-2xl space-y-6 p-6">
        <PageHeader title="Migrate resources" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (pageError) {
    return (
      <div className="mx-auto max-w-2xl space-y-6 p-6">
        <PageHeader title="Migrate resources" />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Failed to load</AlertTitle>
          <AlertDescription>{pageError}</AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => {
            void navigate('/settings/projects')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to projects
        </Button>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6 p-6">
      <div>
        <Button
          variant="ghost"
          size="sm"
          aria-label="Back to projects"
          className="mb-4 -ml-2 gap-1"
          onClick={() => {
            void navigate('/settings/projects')
          }}
        >
          <ArrowLeft className="size-4" />
          Back to projects
        </Button>
        <PageHeader
          title="Migrate resources"
          description="Move prompts, artifacts, blueprints, and feed items between projects."
        />
      </div>

      <StepIndicator currentStep={wizard.step} />

      <Separator />

      {wizard.error && (
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertDescription>{wizard.error}</AlertDescription>
        </Alert>
      )}

      {wizard.step === 1 && (
        <SelectSourceStep
          resolvedSourceProject={resolvedSourceProject}
          selectedProjectId={wizard.sourceProjectId}
          onSelect={handleSourceSelect}
          onNext={() => {
            void handleSourceNext()
          }}
          loadingInventory={wizard.loadingInventory}
        />
      )}

      {wizard.step === 2 && wizard.inventory && (
        <SelectResourcesStep
          inventory={wizard.inventory}
          selectedResources={wizard.selectedResources}
          onResourcesChange={handleResourcesChange}
          onBack={() => {
            setWizard(prev => ({ ...prev, step: 1 }))
          }}
          onNext={() => {
            setWizard(prev => ({ ...prev, step: 3 }))
          }}
        />
      )}

      {wizard.step === 3 && (
        <SelectDestinationStep
          sourceProjectId={wizard.sourceProjectId}
          sourceProjectName={wizard.sourceProjectName}
          destinationProjectId={wizard.destinationProjectId}
          conflictPolicy={wizard.conflictPolicy}
          selectedResources={wizard.selectedResources}
          inventoryCounts={inventoryCounts}
          onDestinationSelect={handleDestinationSelect}
          onConflictPolicyChange={handleConflictPolicyChange}
          onBack={() => {
            setWizard(prev => ({ ...prev, step: 2 }))
          }}
          onMigrate={handleMigrateClick}
          migrating={wizard.migrating}
        />
      )}

      {wizard.step === 4 && wizard.result && (
        <ResultStep
          result={wizard.result}
          destinationProjectSlug={wizard.destinationProjectSlug}
          destinationProjectName={wizard.destinationProjectName}
          onDone={handleDone}
        />
      )}

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Migrate resources?"
        description={confirmDescription}
        confirmLabel="Yes, migrate"
        variant="destructive"
        loading={wizard.migrating}
        onConfirm={() => {
          void handleConfirmMigrate()
        }}
      />
    </div>
  )
}
