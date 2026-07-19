import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type {
  ConflictPolicy,
  MigrationInventory,
  MigrationResult,
  ResourceSelections,
} from '@/services/projectMigrationService'
import type { Project } from '@/services/projectService'

// The four wizard steps have their own suites (migration/*.test.tsx) — stub
// them as probes that echo the props the orchestrator hands down and expose
// buttons for the callbacks, so this file tests ONLY the ProjectMigrate
// wiring: step transitions, inventory loading, selection plumbing, and the
// confirm-before-migrate guard (the ConfirmDialog stays real).
jest.mock('@/pages/settings/projects/migration/SelectSourceStep', () => ({
  SelectSourceStep: ({
    resolvedSourceProject,
    selectedProjectId,
    onSelect,
    onNext,
    loadingInventory,
  }: {
    resolvedSourceProject: Project | null
    selectedProjectId: string
    onSelect: (project: Project) => void
    onNext: () => void
    loadingInventory: boolean
  }) => (
    <div data-testid="source-step">
      <span data-testid="source-resolved-name">
        {resolvedSourceProject?.name ?? ''}
      </span>
      <span data-testid="source-selected-id">{selectedProjectId}</span>
      <span data-testid="source-loading-inventory">
        {String(loadingInventory)}
      </span>
      <button
        type="button"
        onClick={() => {
          onSelect({
            id: 'proj-other',
            slug: 'other-project',
            name: 'Other Project',
          } as Project)
        }}
      >
        probe-select-other-source
      </button>
      <button type="button" onClick={onNext}>
        probe-source-next
      </button>
    </div>
  ),
}))

jest.mock('@/pages/settings/projects/migration/SelectResourcesStep', () => ({
  SelectResourcesStep: ({
    inventory,
    selectedResources,
    onResourcesChange,
    onBack,
    onNext,
  }: {
    inventory: MigrationInventory
    selectedResources: ResourceSelections
    onResourcesChange: (resources: ResourceSelections) => void
    onBack: () => void
    onNext: () => void
  }) => (
    <div data-testid="resources-step">
      <span data-testid="resources-inventory">
        prompts:{inventory.prompts?.count ?? 0} artifacts:
        {inventory.artifacts?.count ?? 0} blueprints:
        {inventory.blueprints?.count ?? 0} feed_items:
        {inventory.feed_items?.count ?? 0}
      </span>
      <span data-testid="resources-selected">
        {JSON.stringify(selectedResources)}
      </span>
      <button
        type="button"
        onClick={() => {
          onResourcesChange({
            prompts: { all: false, ids: ['prompt-1'] },
            artifacts: { all: true, ids: [] },
            blueprints: { all: false, ids: [] },
            feed_items: { all: false, ids: [] },
          })
        }}
      >
        probe-pick-resources
      </button>
      <button type="button" onClick={onBack}>
        probe-resources-back
      </button>
      <button type="button" onClick={onNext}>
        probe-resources-next
      </button>
    </div>
  ),
}))

jest.mock('@/pages/settings/projects/migration/SelectDestinationStep', () => ({
  SelectDestinationStep: ({
    sourceProjectName,
    destinationProjectId,
    conflictPolicy,
    selectedResources,
    inventoryCounts,
    onDestinationSelect,
    onConflictPolicyChange,
    onBack,
    onMigrate,
    migrating,
  }: {
    sourceProjectId: string
    sourceProjectName: string
    destinationProjectId: string
    conflictPolicy: ConflictPolicy
    selectedResources: ResourceSelections
    inventoryCounts: Record<string, number>
    onDestinationSelect: (project: Project) => void
    onConflictPolicyChange: (policy: ConflictPolicy) => void
    onBack: () => void
    onMigrate: () => void
    migrating: boolean
  }) => (
    <div data-testid="destination-step">
      <span data-testid="destination-source-name">{sourceProjectName}</span>
      <span data-testid="destination-selected-id">{destinationProjectId}</span>
      <span data-testid="destination-policy">{conflictPolicy}</span>
      <span data-testid="destination-selected-resources">
        {JSON.stringify(selectedResources)}
      </span>
      <span data-testid="destination-inventory-counts">
        {JSON.stringify(inventoryCounts)}
      </span>
      <span data-testid="destination-migrating">{String(migrating)}</span>
      <button
        type="button"
        onClick={() => {
          onDestinationSelect({
            id: 'proj-dest',
            slug: 'destination-project',
            name: 'Destination Project',
          } as Project)
        }}
      >
        probe-select-destination
      </button>
      <button
        type="button"
        onClick={() => {
          onConflictPolicyChange('overwrite')
        }}
      >
        probe-policy-overwrite
      </button>
      <button type="button" onClick={onBack}>
        probe-destination-back
      </button>
      <button type="button" onClick={onMigrate}>
        probe-migrate
      </button>
    </div>
  ),
}))

jest.mock('@/pages/settings/projects/migration/ResultStep', () => ({
  ResultStep: ({
    result,
    destinationProjectName,
    onDone,
  }: {
    result: MigrationResult
    destinationProjectSlug: string
    destinationProjectName: string
    onDone: () => void
  }) => (
    <div data-testid="result-step">
      <span data-testid="result-destination-name">
        {destinationProjectName}
      </span>
      <span data-testid="result-migrated">
        {JSON.stringify(result.migrated)}
      </span>
      <button type="button" onClick={onDone}>
        probe-done
      </button>
    </div>
  ),
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProject: jest.fn(),
  },
}))

jest.mock('@/services/projectMigrationService', () => ({
  projectMigrationService: {
    getInventory: jest.fn(),
    migrate: jest.fn(),
  },
}))

// Stable identity: a fresh currentTeam object per render would re-trigger the
// page's load effect forever (it depends on currentTeam).
const mockCurrentTeam = { id: 'team-1', name: 'Test Team', permissions: [] }
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: mockCurrentTeam,
    teams: [mockCurrentTeam],
    isLoading: false,
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn() as () => Promise<void>,
  }),
}))

const mockShowSuccess = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: mockShowSuccess, showError: jest.fn() }),
  useAnalytics: () => ({ trackEvent: jest.fn() }),
}))

const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import { projectMigrationService } from '@/services/projectMigrationService'
import { projectService } from '@/services/projectService'

import { ProjectMigrate } from '../ProjectMigrate'

const sourceProject: Project = {
  id: 'proj-src',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'Alpha Project',
  slug: 'alpha-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
}

const inventory: MigrationInventory = {
  prompts: { count: 2, items: [{ id: 'prompt-1', name: 'Prompt one' }] },
  artifacts: { count: 3 },
  blueprints: { count: 0 },
  feed_items: { count: 1 },
}

const migrationResult: MigrationResult = {
  migrated: { prompts: 1, artifacts: 3, blueprints: 0, feed_items: 0 },
  skipped: {},
  failed: {},
  source_project_name: 'Alpha Project',
  destination_project_name: 'Destination Project',
}

const pickedSelection: ResourceSelections = {
  prompts: { all: false, ids: ['prompt-1'] },
  artifacts: { all: true, ids: [] },
  blueprints: { all: false, ids: [] },
  feed_items: { all: false, ids: [] },
}

function renderMigrate() {
  return render(
    <MemoryRouter initialEntries={['/settings/projects/alpha-project/migrate']}>
      <Routes>
        <Route
          path="/settings/projects/:slug/migrate"
          element={<ProjectMigrate />}
        />
        <Route
          path="/settings/projects"
          element={<div data-testid="projects-probe">Projects list probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

/** Drive the wizard from the freshly rendered page to the destination step. */
async function goToDestinationStep(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByTestId('source-step')
  await user.click(screen.getByRole('button', { name: 'probe-source-next' }))
  await screen.findByTestId('resources-step')
  await user.click(screen.getByRole('button', { name: 'probe-pick-resources' }))
  await user.click(screen.getByRole('button', { name: 'probe-resources-next' }))
  await screen.findByTestId('destination-step')
  await user.click(
    screen.getByRole('button', { name: 'probe-select-destination' })
  )
}

describe('ProjectMigrate wizard orchestrator', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    ;(projectService.getProject as jest.Mock).mockResolvedValue(sourceProject)
    ;(projectMigrationService.getInventory as jest.Mock).mockResolvedValue(
      inventory
    )
    ;(projectMigrationService.migrate as jest.Mock).mockResolvedValue(
      migrationResult
    )
  })

  describe('source resolution', () => {
    it('shows a spinner while the source project is loading', () => {
      ;(projectService.getProject as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderMigrate()

      expect(screen.getByText('Migrate resources')).toBeInTheDocument()
      expect(screen.getByRole('status')).toBeInTheDocument()
      expect(screen.queryByTestId('source-step')).not.toBeInTheDocument()
    })

    it('resolves the source project from the URL slug and pre-selects it', async () => {
      renderMigrate()

      await screen.findByTestId('source-step')
      expect(projectService.getProject).toHaveBeenCalledWith(
        'team-1',
        'alpha-project'
      )
      expect(screen.getByTestId('source-resolved-name')).toHaveTextContent(
        'Alpha Project'
      )
      expect(screen.getByTestId('source-selected-id')).toHaveTextContent(
        'proj-src'
      )
    })

    it('shows the page error state with a back action when resolution fails', async () => {
      ;(projectService.getProject as jest.Mock).mockRejectedValue(
        new Error('project gone')
      )

      renderMigrate()

      await screen.findByText('Failed to load')
      expect(screen.getByText('project gone')).toBeInTheDocument()
      expect(mockHandleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load projects'
      )
      expect(screen.queryByTestId('source-step')).not.toBeInTheDocument()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Back to projects/ }))
      expect(screen.getByTestId('projects-probe')).toBeInTheDocument()
    })
  })

  describe('inventory step', () => {
    it('loads the inventory on source Next and shows it on the resources step', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await screen.findByTestId('source-step')
      await user.click(
        screen.getByRole('button', { name: 'probe-source-next' })
      )

      await screen.findByTestId('resources-step')
      expect(projectMigrationService.getInventory).toHaveBeenCalledWith(
        'team-1',
        'proj-src'
      )
      expect(screen.getByTestId('resources-inventory')).toHaveTextContent(
        'prompts:2 artifacts:3 blueprints:0 feed_items:1'
      )
    })

    it('reloads the inventory for a newly selected source project', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await screen.findByTestId('source-step')
      await user.click(
        screen.getByRole('button', { name: 'probe-select-other-source' })
      )
      await user.click(
        screen.getByRole('button', { name: 'probe-source-next' })
      )

      await screen.findByTestId('resources-step')
      expect(projectMigrationService.getInventory).toHaveBeenCalledWith(
        'team-1',
        'proj-other'
      )
    })

    it('stays on the source step and surfaces the error when the inventory fetch fails', async () => {
      ;(projectMigrationService.getInventory as jest.Mock).mockRejectedValue(
        new Error('inventory unavailable')
      )

      renderMigrate()
      const user = userEvent.setup()

      await screen.findByTestId('source-step')
      await user.click(
        screen.getByRole('button', { name: 'probe-source-next' })
      )

      await screen.findByText('inventory unavailable')
      expect(screen.getByTestId('source-step')).toBeInTheDocument()
      expect(screen.queryByTestId('resources-step')).not.toBeInTheDocument()
      expect(mockHandleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load migration inventory'
      )
    })
  })

  describe('selection plumbing', () => {
    it('hands the per-type selection and inventory counts to the destination step', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await goToDestinationStep(user)

      expect(
        screen.getByTestId('destination-selected-resources')
      ).toHaveTextContent(JSON.stringify(pickedSelection))
      expect(
        screen.getByTestId('destination-inventory-counts')
      ).toHaveTextContent(
        JSON.stringify({
          prompts: 2,
          artifacts: 3,
          blueprints: 0,
          feed_items: 1,
        })
      )
      expect(screen.getByTestId('destination-source-name')).toHaveTextContent(
        'Alpha Project'
      )
      expect(screen.getByTestId('destination-policy')).toHaveTextContent('skip')
    })

    it('supports stepping back from resources to source', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await screen.findByTestId('source-step')
      await user.click(
        screen.getByRole('button', { name: 'probe-source-next' })
      )
      await screen.findByTestId('resources-step')
      await user.click(
        screen.getByRole('button', { name: 'probe-resources-back' })
      )

      expect(screen.getByTestId('source-step')).toBeInTheDocument()
    })
  })

  describe('confirmation guard (destructive action)', () => {
    it('does NOT call migrate on Migrate click — only opens the confirm dialog', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await goToDestinationStep(user)
      await user.click(screen.getByRole('button', { name: 'probe-migrate' }))

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Migrate resources?')).toBeInTheDocument()
      // The destructive-action guard: nothing has been migrated yet.
      expect(projectMigrationService.migrate).not.toHaveBeenCalled()
    })

    it('never calls migrate when the dialog is cancelled', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await goToDestinationStep(user)
      await user.click(screen.getByRole('button', { name: 'probe-migrate' }))

      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      })
      expect(projectMigrationService.migrate).not.toHaveBeenCalled()
      expect(screen.getByTestId('destination-step')).toBeInTheDocument()
    })

    it('warns about permanent replacement when the overwrite policy is chosen', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await goToDestinationStep(user)
      await user.click(
        screen.getByRole('button', { name: 'probe-policy-overwrite' })
      )
      await user.click(screen.getByRole('button', { name: 'probe-migrate' }))

      const dialog = await screen.findByRole('alertdialog')
      expect(
        within(dialog).getByText(/permanently replaced/)
      ).toBeInTheDocument()
    })
  })

  describe('migrate execution', () => {
    it('migrates with the accumulated wizard state after explicit confirmation and shows the result', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await goToDestinationStep(user)
      await user.click(screen.getByRole('button', { name: 'probe-migrate' }))
      const dialog = await screen.findByRole('alertdialog')
      await user.click(
        within(dialog).getByRole('button', { name: 'Yes, migrate' })
      )

      await waitFor(() => {
        expect(projectMigrationService.migrate).toHaveBeenCalledWith(
          'team-1',
          'proj-src',
          {
            destination_project_id: 'proj-dest',
            resources: pickedSelection,
            conflict_policy: 'skip',
          }
        )
      })

      // Success feedback + result step with the per-type counts.
      await waitFor(() => {
        expect(mockShowSuccess).toHaveBeenCalledWith(
          'Resources migrated successfully',
          'Migration complete'
        )
      })
      await screen.findByTestId('result-step')
      expect(screen.getByTestId('result-migrated')).toHaveTextContent(
        JSON.stringify(migrationResult.migrated)
      )
      expect(screen.getByTestId('result-destination-name')).toHaveTextContent(
        'Destination Project'
      )

      // Done navigates back to the projects list.
      await user.click(screen.getByRole('button', { name: 'probe-done' }))
      expect(screen.getByTestId('projects-probe')).toBeInTheDocument()
    })

    it('sends the overwrite policy when it was selected', async () => {
      renderMigrate()
      const user = userEvent.setup()

      await goToDestinationStep(user)
      await user.click(
        screen.getByRole('button', { name: 'probe-policy-overwrite' })
      )
      await user.click(screen.getByRole('button', { name: 'probe-migrate' }))
      const dialog = await screen.findByRole('alertdialog')
      await user.click(
        within(dialog).getByRole('button', { name: 'Yes, migrate' })
      )

      await waitFor(() => {
        expect(projectMigrationService.migrate).toHaveBeenCalledWith(
          'team-1',
          'proj-src',
          expect.objectContaining({ conflict_policy: 'overwrite' })
        )
      })
    })

    it('surfaces a migrate failure and stays on the destination step', async () => {
      ;(projectMigrationService.migrate as jest.Mock).mockRejectedValue(
        new Error('destination project not found in team')
      )

      renderMigrate()
      const user = userEvent.setup()

      await goToDestinationStep(user)
      await user.click(screen.getByRole('button', { name: 'probe-migrate' }))
      const dialog = await screen.findByRole('alertdialog')
      await user.click(
        within(dialog).getByRole('button', { name: 'Yes, migrate' })
      )

      await screen.findByText('destination project not found in team')
      expect(mockHandleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Migration failed'
      )
      expect(screen.getByTestId('destination-step')).toBeInTheDocument()
      expect(screen.queryByTestId('result-step')).not.toBeInTheDocument()
      expect(screen.getByTestId('destination-migrating')).toHaveTextContent(
        'false'
      )
    })
  })
})
