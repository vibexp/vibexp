import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import type {
  FreshnessRule,
  TeamFreshnessSettings,
} from '@/services/freshnessService'
import type { Team } from '@/services/teamService'

// usePermissions is deliberately NOT mocked — it reads the permissions off the
// team the page passes it, so the fixtures below exercise the real gating.
const mockUseTeam = vi.hoisted(() => vi.fn())

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/freshnessService', () => ({
  freshnessService: {
    getRules: vi.fn(),
    createRule: vi.fn(),
    updateRule: vi.fn(),
    deleteRule: vi.fn(),
    getSettings: vi.fn(),
    updateSettings: vi.fn(),
    resetSettings: vi.fn(),
  },
}))

vi.mock('@/services/projectService', () => ({
  projectService: { getProjects: vi.fn() },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { freshnessService } from '@/services/freshnessService'
import { projectService } from '@/services/projectService'

import { FreshnessSettings } from '../FreshnessSettings'

const mockedFreshness = vi.mocked(freshnessService)
const mockedProjects = vi.mocked(projectService)

const settings = (
  overrides: Partial<TeamFreshnessSettings> = {}
): TeamFreshnessSettings => ({
  source: 'instance',
  interval_seconds: 86400,
  reversibility_enabled: true,
  defaults: { interval_seconds: 86400, reversibility_enabled: true },
  ...overrides,
})

const rule = (overrides: Partial<FreshnessRule> = {}): FreshnessRule => ({
  id: 'rule-1',
  team_id: 'team-1',
  project_id: null,
  resource_types: ['artifact'],
  mediums: [],
  threshold_days: 90,
  enabled: true,
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
  ...overrides,
})

// Module-stable context objects — a fresh `currentTeam` per render would loop
// any effect keyed on its identity.
const adminTeam = {
  currentTeam: {
    id: 'team-1',
    name: 'Test Team',
    permissions: ['team.settings.update'],
  },
  teams: [{ id: 'team-1', name: 'Test Team' }],
  isLoading: false,
  setCurrentTeam: vi.fn(),
  refreshTeams: vi.fn() as () => Promise<void>,
}

const memberTeam = {
  ...adminTeam,
  currentTeam: {
    id: 'team-1',
    name: 'Test Team',
    // A member holds no team.settings.update — gating must fail closed.
    permissions: ['resource.create'],
  },
}

const asTeam = (ctx: typeof adminTeam) => ctx.currentTeam as unknown as Team

const renderPage = (team: Team = asTeam(adminTeam)) =>
  render(
    <MemoryRouter>
      <FreshnessSettings team={team} />
    </MemoryRouter>
  )

const waitForLoaded = async () => {
  await waitFor(() => {
    expect(screen.getByText('Evaluation')).toBeInTheDocument()
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue(adminTeam)
  mockedFreshness.getSettings.mockResolvedValue(settings())
  mockedFreshness.getRules.mockResolvedValue([])
  mockedProjects.getProjects.mockResolvedValue({
    projects: [
      {
        id: 'project-1',
        name: 'Marketing',
      },
    ],
    total_count: 1,
    page: 1,
    per_page: 10,
    total_pages: 1,
  } as Awaited<ReturnType<typeof projectService.getProjects>>)
})

describe('FreshnessSettings', () => {
  it('loads the settings and rules for the team it was given', async () => {
    renderPage()
    await waitForLoaded()

    expect(mockedFreshness.getSettings).toHaveBeenCalledWith('team-1')
    expect(mockedFreshness.getRules).toHaveBeenCalledWith('team-1')
  })

  it('shows an explanatory empty state when the team has no rules', async () => {
    renderPage()
    await waitForLoaded()

    expect(screen.getByText('No freshness rules')).toBeInTheDocument()
    expect(
      screen.getByText(/Nothing is flagged until you add one/)
    ).toBeInTheDocument()
  })

  it('renders each rule as a readable sentence naming its project', async () => {
    mockedFreshness.getRules.mockResolvedValue([
      rule({ project_id: 'project-1', threshold_days: 30 }),
    ])
    renderPage()
    await waitForLoaded()

    await waitFor(() => {
      expect(
        screen.getByText(
          'Artifacts in Marketing not accessed via any medium for 30 days'
        )
      ).toBeInTheDocument()
    })
  })

  it('marks a disabled rule as such', async () => {
    mockedFreshness.getRules.mockResolvedValue([rule({ enabled: false })])
    renderPage()
    await waitForLoaded()

    await waitFor(() => {
      expect(screen.getByText('Disabled')).toBeInTheDocument()
    })
  })

  it('reports the settings as inherited until the team overrides them', async () => {
    renderPage()
    await waitForLoaded()

    expect(screen.getByText('Using instance defaults')).toBeInTheDocument()
  })

  describe('permission gating', () => {
    it('offers the write affordances to a holder of team.settings.update', async () => {
      mockedFreshness.getRules.mockResolvedValue([rule()])
      renderPage()
      await waitForLoaded()

      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: 'Edit rule' })
        ).toBeInTheDocument()
      })
      expect(
        screen.getByRole('button', { name: 'Delete rule' })
      ).toBeInTheDocument()
      expect(screen.getByTestId('create-rule-button')).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /save evaluation settings/i })
      ).toBeInTheDocument()
    })

    it('hides every write affordance from a member without it', async () => {
      mockUseTeam.mockReturnValue(memberTeam)
      mockedFreshness.getRules.mockResolvedValue([rule()])
      renderPage(asTeam(memberTeam))
      await waitForLoaded()

      await waitFor(() => {
        expect(
          screen.getByText(/not accessed via any medium/)
        ).toBeInTheDocument()
      })
      expect(
        screen.queryByRole('button', { name: 'Edit rule' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Delete rule' })
      ).not.toBeInTheDocument()
      expect(screen.queryByTestId('create-rule-button')).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /save evaluation settings/i })
      ).not.toBeInTheDocument()
      expect(
        screen.getByText(/Only team owners and admins can change these/)
      ).toBeInTheDocument()
    })

    it('still shows a member the rules, because they are team-wide policy', async () => {
      mockUseTeam.mockReturnValue(memberTeam)
      mockedFreshness.getRules.mockResolvedValue([rule()])
      renderPage(asTeam(memberTeam))
      await waitForLoaded()

      await waitFor(() => {
        expect(screen.getByTestId('freshness-rule-row')).toBeInTheDocument()
      })
    })

    it('omits the empty-state create action for a member', async () => {
      mockUseTeam.mockReturnValue(memberTeam)
      renderPage(asTeam(memberTeam))
      await waitForLoaded()

      expect(screen.getByText('No freshness rules')).toBeInTheDocument()
      expect(screen.queryByTestId('create-rule-button')).not.toBeInTheDocument()
    })
  })

  describe('evaluation settings', () => {
    it('persists the chosen interval and reversibility through the settings endpoint', async () => {
      const user = userEvent.setup()
      mockedFreshness.updateSettings.mockResolvedValue(
        settings({ source: 'team', interval_seconds: 3600 })
      )
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByLabelText('Hourly'))
      await user.click(
        screen.getByRole('button', { name: /save evaluation settings/i })
      )

      await waitFor(() => {
        expect(mockedFreshness.updateSettings).toHaveBeenCalledWith('team-1', {
          interval_seconds: 3600,
          reversibility_enabled: true,
        })
      })
    })

    it('sends reversibility off when the toggle is switched off', async () => {
      const user = userEvent.setup()
      mockedFreshness.updateSettings.mockResolvedValue(
        settings({ source: 'team', reversibility_enabled: false })
      )
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByRole('switch'))
      await user.click(
        screen.getByRole('button', { name: /save evaluation settings/i })
      )

      await waitFor(() => {
        expect(mockedFreshness.updateSettings).toHaveBeenCalledWith('team-1', {
          interval_seconds: 86400,
          reversibility_enabled: false,
        })
      })
    })

    it('keeps the save button inert until something actually changes', async () => {
      renderPage()
      await waitForLoaded()

      expect(
        screen.getByRole('button', { name: /save evaluation settings/i })
      ).toBeDisabled()
    })

    it('offers a non-preset stored interval rather than rounding it away', async () => {
      // A team configured through the API directly must not have its cadence
      // silently changed by opening this page.
      mockedFreshness.getSettings.mockResolvedValue(
        settings({ source: 'team', interval_seconds: 259200 })
      )
      renderPage()
      await waitForLoaded()

      expect(screen.getByLabelText('Every 3 days')).toBeChecked()
    })

    it('keeps the non-preset option selectable after a preset is chosen', async () => {
      // Keying the extra option off the live selection instead of the stored
      // value would drop it here, leaving nothing that can restore the team's
      // original cadence without a page reload.
      const user = userEvent.setup()
      mockedFreshness.getSettings.mockResolvedValue(
        settings({ source: 'team', interval_seconds: 259200 })
      )
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByLabelText('Weekly'))

      expect(screen.getByLabelText('Every 3 days')).toBeInTheDocument()
      await user.click(screen.getByLabelText('Every 3 days'))
      expect(screen.getByLabelText('Every 3 days')).toBeChecked()
    })

    it('resets an overridden team back to the instance defaults', async () => {
      const user = userEvent.setup()
      mockedFreshness.getSettings
        .mockResolvedValueOnce(
          settings({ source: 'team', interval_seconds: 3600 })
        )
        .mockResolvedValueOnce(settings())
      mockedFreshness.resetSettings.mockResolvedValue(undefined)
      renderPage()
      await waitForLoaded()

      await user.click(
        screen.getByRole('button', { name: /reset to defaults/i })
      )

      await waitFor(() => {
        expect(mockedFreshness.resetSettings).toHaveBeenCalledWith('team-1')
      })
      // The refetch is what restores the displayed values.
      await waitFor(() => {
        expect(screen.getByLabelText('Daily')).toBeChecked()
      })
    })

    it('offers no reset while the team is still inheriting', async () => {
      renderPage()
      await waitForLoaded()

      expect(
        screen.queryByRole('button', { name: /reset to defaults/i })
      ).not.toBeInTheDocument()
    })

    it('hides the reset control from a member', async () => {
      mockUseTeam.mockReturnValue(memberTeam)
      mockedFreshness.getSettings.mockResolvedValue(
        settings({ source: 'team' })
      )
      renderPage(asTeam(memberTeam))
      await waitForLoaded()

      expect(
        screen.queryByRole('button', { name: /reset to defaults/i })
      ).not.toBeInTheDocument()
    })

    it('reports a failed reset without claiming success', async () => {
      const user = userEvent.setup()
      mockedFreshness.getSettings.mockResolvedValue(
        settings({ source: 'team' })
      )
      mockedFreshness.resetSettings.mockRejectedValue(new Error('Reset failed'))
      renderPage()
      await waitForLoaded()

      await user.click(
        screen.getByRole('button', { name: /reset to defaults/i })
      )

      await waitFor(() => {
        expect(screen.getByText('Reset failed')).toBeInTheDocument()
      })
    })

    it('surfaces a failed save without clearing the form', async () => {
      const user = userEvent.setup()
      mockedFreshness.updateSettings.mockRejectedValue(
        new Error('interval_seconds must be at least 3600')
      )
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByLabelText('Weekly'))
      await user.click(
        screen.getByRole('button', { name: /save evaluation settings/i })
      )

      await waitFor(() => {
        expect(
          screen.getByText('interval_seconds must be at least 3600')
        ).toBeInTheDocument()
      })
      expect(screen.getByLabelText('Weekly')).toBeChecked()
    })
  })

  describe('rule CRUD', () => {
    it('creates a rule from the dialog and refetches', async () => {
      const user = userEvent.setup()
      mockedFreshness.createRule.mockResolvedValue(rule())
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByTestId('create-rule-button'))
      await user.click(screen.getByLabelText('Prompts'))
      await user.click(screen.getByLabelText('CLI'))
      await user.selectOptions(
        screen.getByTestId('rule-project-select'),
        'project-1'
      )
      await user.clear(screen.getByLabelText('Threshold (days)'))
      await user.type(screen.getByLabelText('Threshold (days)'), '45')
      await user.click(screen.getByTestId('submit-rule-button'))

      await waitFor(() => {
        expect(mockedFreshness.createRule).toHaveBeenCalledWith('team-1', {
          project_id: 'project-1',
          resource_types: ['prompt'],
          mediums: ['cli'],
          threshold_days: 45,
          enabled: true,
        })
      })
      // Two calls: the initial load and the silent refetch after the write.
      expect(mockedFreshness.getRules).toHaveBeenCalledTimes(2)
    })

    it('keeps unsaved evaluation edits when a rule write refetches', async () => {
      // The refetch after a rule write must not re-read the settings: doing so
      // would overwrite the evaluation form and silently discard the interval
      // the user had picked but not yet saved.
      const user = userEvent.setup()
      mockedFreshness.createRule.mockResolvedValue(rule())
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByLabelText('Weekly'))

      await user.click(screen.getByTestId('create-rule-button'))
      await user.click(screen.getByLabelText('Artifacts'))
      await user.click(screen.getByTestId('submit-rule-button'))

      await waitFor(() => {
        expect(mockedFreshness.createRule).toHaveBeenCalled()
      })
      expect(screen.getByLabelText('Weekly')).toBeChecked()
      // Only the mount read the settings.
      expect(mockedFreshness.getSettings).toHaveBeenCalledTimes(1)
    })

    it('edits an existing rule through a full replacement', async () => {
      const user = userEvent.setup()
      mockedFreshness.getRules.mockResolvedValue([
        rule({ project_id: 'project-1', mediums: ['web'], threshold_days: 30 }),
      ])
      mockedFreshness.updateRule.mockResolvedValue(rule())
      renderPage()
      await waitForLoaded()

      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: 'Edit rule' })
        ).toBeInTheDocument()
      })
      await user.click(screen.getByRole('button', { name: 'Edit rule' }))

      // The dialog opens seeded from the rule.
      expect(screen.getByLabelText('Artifacts')).toBeChecked()
      expect(screen.getByLabelText('Web app')).toBeChecked()

      await user.click(screen.getByLabelText('Memories'))
      await user.click(screen.getByTestId('submit-rule-button'))

      await waitFor(() => {
        expect(mockedFreshness.updateRule).toHaveBeenCalledWith(
          'team-1',
          'rule-1',
          {
            project_id: 'project-1',
            resource_types: ['artifact', 'memory'],
            mediums: ['web'],
            threshold_days: 30,
            enabled: true,
          }
        )
      })
    })

    it('deletes a rule after the confirmation', async () => {
      const user = userEvent.setup()
      mockedFreshness.getRules.mockResolvedValue([rule()])
      mockedFreshness.deleteRule.mockResolvedValue(undefined)
      renderPage()
      await waitForLoaded()

      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: 'Delete rule' })
        ).toBeInTheDocument()
      })
      await user.click(screen.getByRole('button', { name: 'Delete rule' }))

      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => {
        expect(mockedFreshness.deleteRule).toHaveBeenCalledWith(
          'team-1',
          'rule-1'
        )
      })
    })

    it('blocks submission until a resource type is chosen', async () => {
      const user = userEvent.setup()
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByTestId('create-rule-button'))

      expect(screen.getByTestId('submit-rule-button')).toBeDisabled()
      expect(
        screen.getByText('Select at least one resource type.')
      ).toBeInTheDocument()
      expect(mockedFreshness.createRule).not.toHaveBeenCalled()
    })

    it('blocks submission on a threshold the server would reject', async () => {
      const user = userEvent.setup()
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByTestId('create-rule-button'))
      await user.click(screen.getByLabelText('Artifacts'))
      await user.clear(screen.getByLabelText('Threshold (days)'))
      await user.type(screen.getByLabelText('Threshold (days)'), '0')

      expect(screen.getByTestId('submit-rule-button')).toBeDisabled()
      expect(
        screen.getByText('Threshold must be at least 1 day.')
      ).toBeInTheDocument()
    })

    it('surfaces a server rejection in the dialog and keeps the input', async () => {
      const user = userEvent.setup()
      mockedFreshness.createRule.mockRejectedValue(
        new Error('threshold_days exceeds the maximum')
      )
      renderPage()
      await waitForLoaded()

      await user.click(screen.getByTestId('create-rule-button'))
      await user.click(screen.getByLabelText('Blueprints'))
      await user.click(screen.getByTestId('submit-rule-button'))

      await waitFor(() => {
        expect(
          screen.getByText('threshold_days exceeds the maximum')
        ).toBeInTheDocument()
      })
      // The dialog stays open so the user can fix the value they typed.
      expect(screen.getByLabelText('Blueprints')).toBeChecked()
    })
  })

  it('renders the rules even when the project lookup fails', async () => {
    // Projects only name the rules' scope; losing them must not fail the page.
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    mockedProjects.getProjects.mockRejectedValue(new Error('boom'))
    mockedFreshness.getRules.mockResolvedValue([
      rule({ project_id: 'project-1' }),
    ])
    renderPage()
    await waitForLoaded()

    await waitFor(() => {
      expect(screen.getByText(/in a project/)).toBeInTheDocument()
    })
    consoleError.mockRestore()
  })

  it('shows the load error when the settings request fails', async () => {
    mockedFreshness.getSettings.mockRejectedValue(new Error('Service down'))
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Service down')).toBeInTheDocument()
    })
  })
})
