import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { Team } from '@/services/teamService'

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

import { SourceTeamPicker } from '../SourceTeamPicker'

const makeTeam = (id: string, name: string, permissions: string[] = []): Team =>
  ({
    id,
    owner_id: 'owner-1',
    name,
    slug: name.toLowerCase().replace(/\s+/g, '-'),
    description: '',
    is_personal: false,
    permissions,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }) as unknown as Team

const destination = makeTeam('team-1', 'Destination Team')
const alpha = makeTeam('team-2', 'Alpha Team', ['team.update'])
const beta = makeTeam('team-3', 'Beta Team', [])

// cmdk needs browser APIs jsdom does not provide.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: destination,
    teams: [destination, alpha, beta],
    isLoading: false,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn(),
  })
})

function renderPicker(
  props: Partial<{ canCopyFrom: (t: Team) => boolean }> = {}
) {
  const onChange = vi.fn()
  render(
    <SourceTeamPicker
      destinationTeam={destination}
      value={null}
      onChange={onChange}
      open
      onOpenChange={vi.fn()}
      {...props}
    />
  )
  return { onChange }
}

it('lists the other teams and excludes the destination', async () => {
  renderPicker()

  expect(await screen.findByText('Alpha Team')).toBeInTheDocument()
  expect(screen.getByText('Beta Team')).toBeInTheDocument()
  // The destination would be a 400 (source must differ), so it is never offered.
  expect(screen.queryByText('Destination Team')).not.toBeInTheDocument()
  expect(screen.getAllByTestId('source-team-option')).toHaveLength(2)
})

it('applies canCopyFrom so a page can require a permission on the source', async () => {
  renderPicker({ canCopyFrom: t => t.permissions.includes('team.update') })

  expect(await screen.findByText('Alpha Team')).toBeInTheDocument()
  expect(screen.queryByText('Beta Team')).not.toBeInTheDocument()
})

it('reports the picked team to the caller', async () => {
  const user = userEvent.setup()
  const { onChange } = renderPicker()

  await user.click(await screen.findByText('Beta Team'))

  expect(onChange).toHaveBeenCalledWith(beta)
})

it('shows an empty state when the user belongs to no other team', async () => {
  mockUseTeam.mockReturnValue({
    currentTeam: destination,
    teams: [destination],
    isLoading: false,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn(),
  })
  renderPicker()

  expect(await screen.findByText('No other teams found.')).toBeInTheDocument()
})
