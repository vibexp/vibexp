import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { TeamSearchSettings } from '@/services/searchSettingsService'
import type { Team } from '@/services/teamService'

// usePermissions is deliberately NOT mocked — it reads the permissions off the
// team the page passes it, so the fixtures below exercise the real gating.
const mockUseTeam = jest.fn()

jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/searchSettingsService', () => ({
  searchSettingsService: {
    getSearchSettings: jest.fn(),
    updateSearchSettings: jest.fn(),
    resetSearchSettings: jest.fn(),
  },
}))

import { searchSettingsService } from '@/services/searchSettingsService'

import { detectPreset } from '../searchRankingPresets'
import { SearchSettings } from '../SearchSettings'

const mockedService = jest.mocked(searchSettingsService)

const BALANCED = {
  recency_ranking_enabled: true,
  rank_weight_relevance: 0.5,
  rank_weight_created: 0.3,
  rank_weight_updated: 0.2,
  rank_half_life_days: 90,
}

const FAVOR_RECENT = {
  recency_ranking_enabled: true,
  rank_weight_relevance: 0.3,
  rank_weight_created: 0.4,
  rank_weight_updated: 0.3,
  rank_half_life_days: 30,
}

const settings = (
  overrides: Partial<TeamSearchSettings> = {}
): TeamSearchSettings => ({
  source: 'instance',
  ...BALANCED,
  instance_defaults: BALANCED,
  rank_candidate_cap: 200,
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
  setCurrentTeam: jest.fn(),
  refreshTeams: jest.fn() as () => Promise<void>,
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

// #540: the page now takes the team TeamScopeLayout resolved from the URL, and
// gates permissions on IT rather than on the ambient currentTeam. Tests pass the
// team explicitly; by default it mirrors whichever context fixture is active.
const asTeam = (ctx: typeof adminTeam) => ctx.currentTeam as unknown as Team

const renderPage = (team: Team = asTeam(adminTeam)) =>
  render(
    <MemoryRouter>
      <SearchSettings team={team} />
    </MemoryRouter>
  )

const openAdvanced = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(screen.getByRole('button', { name: /advanced/i }))
}

beforeEach(() => {
  jest.clearAllMocks()
  mockUseTeam.mockReturnValue(adminTeam)
  mockedService.getSearchSettings.mockResolvedValue(settings())
})

describe('detectPreset', () => {
  it('maps each preset back from its exact values', () => {
    expect(detectPreset(BALANCED)).toBe('balanced')
    expect(detectPreset(FAVOR_RECENT)).toBe('favor-recent')
  })

  it('reads recency-off as Relevance only whatever the weights are', () => {
    expect(
      detectPreset({ ...FAVOR_RECENT, recency_ranking_enabled: false })
    ).toBe('relevance-only')
  })

  it('falls back to Custom for off-preset values', () => {
    expect(detectPreset({ ...BALANCED, rank_half_life_days: 45 })).toBe(
      'custom'
    )
  })

  it('tolerates floating-point drift rather than falling through to Custom', () => {
    // 0.1 + 0.2 === 0.30000000000000004; an === comparison would say "custom".
    expect(detectPreset({ ...BALANCED, rank_weight_created: 0.1 + 0.2 })).toBe(
      'balanced'
    )
  })
})

describe('SearchSettings', () => {
  it('selects the preset matching the loaded values', async () => {
    renderPage()

    expect(
      await screen.findByRole('radio', { name: /balanced/i })
    ).toBeChecked()
    expect(
      screen.getByRole('radio', { name: /favor recent/i })
    ).not.toBeChecked()
    expect(screen.getByRole('radio', { name: /custom/i })).not.toBeChecked()
  })

  it('shows Custom as selected when the values match no preset', async () => {
    mockedService.getSearchSettings.mockResolvedValue(
      settings({ source: 'team', rank_half_life_days: 45 })
    )

    renderPage()

    expect(await screen.findByRole('radio', { name: /custom/i })).toBeChecked()
    expect(screen.getByRole('radio', { name: /balanced/i })).not.toBeChecked()
  })

  it('badges instance provenance and offers no reset', async () => {
    renderPage()

    expect(
      await screen.findByText('Using instance defaults')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /reset to defaults/i })
    ).not.toBeInTheDocument()
  })

  it('offers reset with a preview of the defaults when the team has its own profile', async () => {
    mockedService.getSearchSettings.mockResolvedValue(
      settings({ source: 'team', ...FAVOR_RECENT })
    )

    renderPage()

    expect(
      await screen.findByRole('button', { name: /reset to defaults/i })
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Using instance defaults')
    ).not.toBeInTheDocument()
    // Previewed from instance_defaults on the same GET — no second call.
    expect(screen.getByText(/half-life 90d/)).toBeInTheDocument()
    expect(mockedService.getSearchSettings).toHaveBeenCalledTimes(1)
  })

  it('saves the preset values as a complete profile', async () => {
    const user = userEvent.setup()
    mockedService.updateSearchSettings.mockResolvedValue(
      settings({ source: 'team', ...FAVOR_RECENT })
    )

    renderPage()

    await user.click(
      await screen.findByRole('radio', { name: /favor recent/i })
    )
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(mockedService.updateSearchSettings).toHaveBeenCalledWith(
        'team-1',
        FAVOR_RECENT
      )
    })
    expect(
      await screen.findByText('Search ranking settings saved.')
    ).toBeInTheDocument()
  })

  it('keeps the tuned weights when switching to Relevance only', async () => {
    const user = userEvent.setup()
    mockedService.updateSearchSettings.mockResolvedValue(
      settings({ source: 'team', recency_ranking_enabled: false })
    )

    renderPage()

    await user.click(
      await screen.findByRole('radio', { name: /relevance only/i })
    )
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(mockedService.updateSearchSettings).toHaveBeenCalledWith(
        'team-1',
        {
          ...BALANCED,
          recency_ranking_enabled: false,
        }
      )
    })
  })

  it('persists edited weights and half-life from Advanced', async () => {
    const user = userEvent.setup()
    mockedService.updateSearchSettings.mockResolvedValue(
      settings({ source: 'team', rank_half_life_days: 45 })
    )

    renderPage()
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: /balanced/i })).toBeChecked()
    })
    await openAdvanced(user)

    const halfLife = screen.getByLabelText(/half-life \(days\)/i)
    await user.clear(halfLife)
    await user.type(halfLife, '45')

    // Tuning off-preset flips the derived radio to Custom.
    expect(screen.getByRole('radio', { name: /custom/i })).toBeChecked()

    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(mockedService.updateSearchSettings).toHaveBeenCalledWith(
        'team-1',
        {
          ...BALANCED,
          rank_half_life_days: 45,
        }
      )
    })
  })

  it('blocks saving an invalid profile instead of posting a 400', async () => {
    const user = userEvent.setup()

    renderPage()
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: /balanced/i })).toBeChecked()
    })
    await openAdvanced(user)

    const halfLife = screen.getByLabelText(/half-life \(days\)/i)
    await user.clear(halfLife)
    await user.type(halfLife, '0')

    expect(
      await screen.findByText(/half-life must be greater than 0/i)
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /save changes/i })).toBeDisabled()
    expect(mockedService.updateSearchSettings).not.toHaveBeenCalled()
  })

  it('resets via DELETE and refetches the now-inherited settings', async () => {
    const user = userEvent.setup()
    mockedService.getSearchSettings
      .mockResolvedValueOnce(settings({ source: 'team', ...FAVOR_RECENT }))
      .mockResolvedValueOnce(settings())
    mockedService.resetSearchSettings.mockResolvedValue(undefined)

    renderPage()

    await user.click(
      await screen.findByRole('button', { name: /reset to defaults/i })
    )

    await waitFor(() => {
      expect(mockedService.resetSearchSettings).toHaveBeenCalledWith('team-1')
    })
    expect(mockedService.getSearchSettings).toHaveBeenCalledTimes(2)
    expect(
      await screen.findByText('Reset to the instance defaults.')
    ).toBeInTheDocument()
    expect(
      await screen.findByText('Using instance defaults')
    ).toBeInTheDocument()
  })

  it('keeps the Advanced disclosure open across a reset', async () => {
    const user = userEvent.setup()
    mockedService.getSearchSettings
      .mockResolvedValueOnce(settings({ source: 'team', ...FAVOR_RECENT }))
      // The refetch must NOT resolve within the same tick: an instantly
      // resolved mock lets React batch `loading` true→false without ever
      // rendering, which would make this test pass even for a full-page
      // spinner. The delay forces the intermediate render this guards against.
      .mockImplementationOnce(
        () =>
          new Promise(resolve => {
            setTimeout(() => {
              resolve(settings())
            }, 20)
          })
      )
    mockedService.resetSearchSettings.mockResolvedValue(undefined)

    renderPage()
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: /favor recent/i })).toBeChecked()
    })
    await openAdvanced(user)
    expect(screen.getByLabelText(/half-life \(days\)/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /reset to defaults/i }))

    // A full-page spinner on refetch would unmount the disclosure and collapse
    // it under the user, losing the panel they had deliberately opened.
    expect(
      await screen.findByText('Reset to the instance defaults.')
    ).toBeInTheDocument()
    expect(screen.getByLabelText(/half-life \(days\)/i)).toHaveValue(90)
  })

  it('does not claim a successful reset when the refetch fails', async () => {
    const user = userEvent.setup()
    mockedService.getSearchSettings
      .mockResolvedValueOnce(settings({ source: 'team', ...FAVOR_RECENT }))
      .mockRejectedValueOnce(new Error('Failed to load search settings'))
    mockedService.resetSearchSettings.mockResolvedValue(undefined)

    renderPage()

    await user.click(
      await screen.findByRole('button', { name: /reset to defaults/i })
    )

    expect(
      await screen.findByText('Failed to load search settings')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Reset to the instance defaults.')
    ).not.toBeInTheDocument()
  })

  it('surfaces a failed save without losing the edit', async () => {
    const user = userEvent.setup()
    mockedService.updateSearchSettings.mockRejectedValue(
      new Error('Failed to save search settings')
    )

    renderPage()

    await user.click(
      await screen.findByRole('radio', { name: /favor recent/i })
    )
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    expect(
      await screen.findByText('Failed to save search settings')
    ).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /favor recent/i })).toBeChecked()
  })

  describe('the candidate-cap pagination note', () => {
    it('renders with the API-supplied cap while recency ranking is on', async () => {
      const user = userEvent.setup()
      mockedService.getSearchSettings.mockResolvedValue(
        settings({ rank_candidate_cap: 350 })
      )

      renderPage()
      await waitFor(() => {
        expect(screen.getByRole('radio', { name: /balanced/i })).toBeChecked()
      })
      await openAdvanced(user)

      const note = screen.getByText(/re-ranks the top/i)
      expect(note).toHaveTextContent(
        "With recency ranking enabled, search re-ranks the top 350 matches — results beyond that aren't reachable via pagination."
      )
    })

    it('is absent when recency ranking is off', async () => {
      const user = userEvent.setup()
      mockedService.getSearchSettings.mockResolvedValue(
        settings({ recency_ranking_enabled: false })
      )

      renderPage()
      await waitFor(() => {
        expect(
          screen.getByRole('radio', { name: /relevance only/i })
        ).toBeChecked()
      })
      await openAdvanced(user)

      expect(screen.queryByText(/re-ranks the top/i)).not.toBeInTheDocument()
    })
  })

  describe('permission gating', () => {
    it('gives a member a read-only view', async () => {
      const user = userEvent.setup()
      mockUseTeam.mockReturnValue(memberTeam)
      mockedService.getSearchSettings.mockResolvedValue(
        settings({ source: 'team', ...FAVOR_RECENT })
      )

      renderPage(asTeam(memberTeam))

      expect(
        await screen.findByText(
          'Only team owners and admins can change these settings.'
        )
      ).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /save changes/i })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /reset to defaults/i })
      ).not.toBeInTheDocument()

      // The effective profile is still visible, just not editable.
      const profile = screen.getByRole('group', { name: 'Ranking profile' })
      expect(
        within(profile).getByRole('radio', { name: /favor recent/i })
      ).toBeChecked()
      expect(
        within(profile).getByRole('radio', { name: /favor recent/i })
      ).toBeDisabled()

      await openAdvanced(user)
      expect(screen.getByLabelText(/half-life \(days\)/i)).toBeDisabled()
    })

    // #540 AC: the permission must resolve against the team in the URL. These
    // two cases fail if the page reverts to `usePermissions()` (ambient), which
    // is exactly the silent degradation the issue flagged - and it would be
    // invisible in production while the layout's sync happens to be up to date.
    it('gates on the URL team, not the ambient team (ambient admin, URL member)', async () => {
      mockUseTeam.mockReturnValue(adminTeam)
      mockedService.getSearchSettings.mockResolvedValue(
        settings({ source: 'team', ...FAVOR_RECENT })
      )

      renderPage(asTeam(memberTeam))

      expect(
        await screen.findByText(
          'Only team owners and admins can change these settings.'
        )
      ).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /save changes/i })
      ).not.toBeInTheDocument()
    })

    it('gates on the URL team, not the ambient team (ambient member, URL admin)', async () => {
      mockUseTeam.mockReturnValue(memberTeam)

      renderPage(asTeam(adminTeam))

      expect(
        await screen.findByRole('button', { name: /save changes/i })
      ).toBeInTheDocument()
      expect(
        screen.queryByText(
          'Only team owners and admins can change these settings.'
        )
      ).not.toBeInTheDocument()
    })

    it('lets an admin edit', async () => {
      renderPage()

      expect(
        await screen.findByRole('button', { name: /save changes/i })
      ).toBeInTheDocument()
      expect(screen.getByRole('radio', { name: /balanced/i })).toBeEnabled()
      expect(
        screen.queryByText(
          'Only team owners and admins can change these settings.'
        )
      ).not.toBeInTheDocument()
    })
  })

  it('reports a failed load', async () => {
    mockedService.getSearchSettings.mockRejectedValue(
      new Error('Failed to load search settings')
    )

    renderPage()

    expect(
      await screen.findByText('Failed to load search settings')
    ).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// #584 — read and write the URL's team, not the ambient one
// ---------------------------------------------------------------------------
// #540 already gated PERMISSIONS on the resolved team; #584 moved the data
// layer onto it too. The mocked TeamContext above reports team-1, so rendering
// with an explicit team-b prop reproduces the cold deep-link divergence: React
// fires child effects before parent effects, so this page's load effect used to
// run while the ambient team was still the previously persisted one.
describe('cold deep-link (#584)', () => {
  const urlTeamB = {
    id: 'team-b',
    name: 'Team B',
    permissions: ['team.settings.update'],
  } as unknown as Team

  it('loads settings for the URL team, never the ambient one', async () => {
    renderPage(urlTeamB)

    await waitFor(() => {
      expect(mockedService.getSearchSettings).toHaveBeenCalledWith('team-b')
    })
    expect(mockedService.getSearchSettings).not.toHaveBeenCalledWith('team-1')
  })
})
