import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { TooltipProvider } from '@/components/ui/tooltip'
import type { Team } from '@/services/teamService'

// Module-stable context: a fresh object per render loops effects keyed on it.
const mockSetCurrentTeam = vi.hoisted(() => vi.fn())
const ctx: {
  teams: Team[]
  currentTeam: Team | null
  isLoading: boolean
} = { teams: [], currentTeam: null, isLoading: false }

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    teams: ctx.teams,
    currentTeam: ctx.currentTeam,
    isLoading: ctx.isLoading,
    setCurrentTeam: mockSetCurrentTeam,
  }),
}))

import { TeamSwitcher } from '../TeamSwitcher'

const makeTeam = (id: string, name: string): Team => ({
  id,
  owner_id: 'owner-1',
  name,
  slug: name.toLowerCase(),
  description: '',
  is_personal: false,
  permissions: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
})

const ENGINEERING = makeTeam('team-a', 'Engineering')
const DESIGN = makeTeam('team-b', 'Design')

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <TeamSwitcher />
    </MemoryRouter>
  )
}

const switcher = () => screen.getByTestId('team-switcher')

describe('TeamSwitcher', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ctx.teams = [ENGINEERING, DESIGN]
    ctx.currentTeam = ENGINEERING
    ctx.isLoading = false
  })

  describe('team-scoped routes stay interactive', () => {
    it.each([
      '/',
      '/search',
      '/prompts',
      '/artifacts',
      '/blueprints',
      '/memories',
      '/feeds',
      '/agents',
    ])('is enabled on %s', path => {
      renderAt(path)
      expect(switcher()).toBeEnabled()
    })

    it('carries no tooltip when interactive', () => {
      const { container } = renderAt('/prompts')
      expect(container.querySelector('span[title]')).toBeNull()
    })
  })

  describe('personal routes disable it', () => {
    it.each([
      '/settings',
      '/settings/api-keys',
      '/settings/activities',
      '/settings/notifications',
      '/notifications',
    ])('is disabled on %s', path => {
      renderAt(path)
      expect(switcher()).toBeDisabled()
    })

    it('explains that the team does not apply', () => {
      const { container } = renderAt('/settings')
      expect(
        container.querySelector('span[title]')?.getAttribute('title')
      ).toBe('Your team does not apply to this page')
    })
  })

  describe('the teams list is personal, its children are not', () => {
    it('is disabled on /teams (exact)', () => {
      renderAt('/teams')
      expect(switcher()).toBeDisabled()
    })

    it('uses the personal explanation on /teams, not the scoped one', () => {
      // `/teams` as a PREFIX would swallow /teams/:id/** and show the wrong
      // reason there; this pair is what pins the exact-match behaviour.
      const { container } = renderAt('/teams')
      expect(
        container.querySelector('span[title]')?.getAttribute('title')
      ).toBe('Your team does not apply to this page')
    })
  })

  describe('/teams/:id/** pins the team from the URL', () => {
    it('is disabled and names the URL team in the tooltip', () => {
      const { container } = renderAt('/teams/team-a/settings')
      expect(switcher()).toBeDisabled()
      expect(
        container.querySelector('span[title]')?.getAttribute('title')
      ).toBe('This page is scoped to Engineering')
    })

    it.each([
      '/teams/team-a',
      '/teams/team-a/analytics',
      '/teams/team-a/projects',
      '/teams/team-a/settings/search',
    ])('is disabled across the subtree (%s)', path => {
      renderAt(path)
      expect(switcher()).toBeDisabled()
    })

    it('gives a different explanation than a personal page', () => {
      const scoped = renderAt('/teams/team-a')
      const scopedTitle = scoped.container
        .querySelector('span[title]')
        ?.getAttribute('title')
      scoped.unmount()

      const personal = renderAt('/settings')
      const personalTitle = personal.container
        .querySelector('span[title]')
        ?.getAttribute('title')

      // Same visual state, different reason - conflating them makes the
      // URL-pinned case read like a bug.
      expect(scopedTitle).not.toBe(personalTitle)
    })

    it('shows Loading rather than the previous team while the scope syncs', () => {
      // TeamScopeLayout syncs currentTeam to :id in an effect that runs AFTER
      // the header renders, so without this the header briefly shows team A
      // under a URL naming team B.
      ctx.currentTeam = ENGINEERING
      renderAt('/teams/team-b/settings')

      expect(screen.getByRole('button', { name: /loading/i })).toBeDisabled()
      expect(screen.queryByTestId('team-switcher')).not.toBeInTheDocument()
      expect(screen.queryByText('Engineering')).not.toBeInTheDocument()
    })

    it('shows the URL team once the sync has settled', () => {
      ctx.currentTeam = DESIGN
      renderAt('/teams/team-b/settings')

      expect(screen.getByTestId('current-team-name')).toHaveTextContent(
        'Design'
      )
      expect(switcher()).toBeDisabled()
    })
  })

  describe('existing early returns are preserved', () => {
    it('renders the loading button while teams load', () => {
      ctx.isLoading = true
      renderAt('/prompts')
      expect(screen.getByRole('button', { name: /loading/i })).toBeDisabled()
    })

    it('renders nothing when the user has no teams', () => {
      ctx.teams = []
      const { container } = renderAt('/prompts')
      expect(container).toBeEmptyDOMElement()
    })
  })

  // The collapsed sidebar has no room for a name: the trigger is the team's
  // initial, named by its label and a tooltip.
  describe('rail variant (collapsed sidebar)', () => {
    function renderRailAt(path: string) {
      return render(
        <MemoryRouter initialEntries={[path]}>
          <TooltipProvider delayDuration={0}>
            <TeamSwitcher variant="rail" />
          </TooltipProvider>
        </MemoryRouter>
      )
    }

    it('shows the initial and names the workspace', () => {
      renderRailAt('/prompts')

      expect(switcher()).toHaveAccessibleName('Workspace: Engineering')
      expect(switcher()).toHaveTextContent('E')
      // The full name stays in the DOM for assistive tech and for tests.
      expect(screen.getByTestId('current-team-name')).toHaveTextContent(
        'Engineering'
      )
    })

    it('puts both the popover and the tooltip state on the real button (#891)', async () => {
      const user = userEvent.setup()
      renderRailAt('/prompts')

      // One element is the trigger for both: a <button> with a box, not a
      // wrapper the popper would measure as 0×0.
      expect(switcher().tagName).toBe('BUTTON')
      expect(switcher()).toHaveAttribute('aria-haspopup', 'dialog')

      await user.hover(switcher())
      await waitFor(() => {
        expect(switcher()).toHaveAttribute('aria-describedby')
      })
      expect(await screen.findByRole('tooltip')).toHaveTextContent(
        'Engineering'
      )
    })

    it('still switches team from the rail', async () => {
      // cmdk scrolls the selected item into view; jsdom has no layout.
      Element.prototype.scrollIntoView = vi.fn()
      const user = userEvent.setup()
      renderRailAt('/prompts')

      await user.click(switcher())
      await user.click(await screen.findByText('Design'))

      expect(mockSetCurrentTeam).toHaveBeenCalledWith(DESIGN)
    })
  })
})
