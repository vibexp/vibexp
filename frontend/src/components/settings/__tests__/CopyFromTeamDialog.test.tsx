import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { Team } from '@/services/teamService'

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

import { CopyFromTeamDialog } from '../CopyFromTeamDialog'

const makeTeam = (id: string, name: string): Team => ({
  id,
  owner_id: 'owner-1',
  name,
  slug: name.toLowerCase().replace(/\s+/g, '-'),
  description: '',
  is_personal: false,
  permissions: [],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
})

const destination = makeTeam('team-1', 'Destination Team')
const alpha = makeTeam('team-2', 'Alpha Team')

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: destination,
    teams: [destination, alpha],
    isLoading: false,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn(),
  })
})

interface Overrides {
  open?: boolean
  submitting?: boolean
  onConfirm?: (team: Team) => void
  onSourceChange?: (team: Team | null) => void
  preview?: React.ReactNode
}

function renderDialog(overrides: Overrides = {}) {
  const onConfirm = overrides.onConfirm ?? vi.fn()
  const onSourceChange = overrides.onSourceChange ?? vi.fn()
  const view = render(
    <CopyFromTeamDialog
      open={overrides.open ?? true}
      onOpenChange={vi.fn()}
      team={destination}
      title="Copy artifact types"
      description="Bring another team's types into this one."
      submitting={overrides.submitting ?? false}
      onConfirm={onConfirm}
      onSourceChange={onSourceChange}
      preview={overrides.preview}
      confirmLabel="Copy types"
    />
  )
  return { ...view, onConfirm, onSourceChange }
}

it('cannot confirm until a source team is chosen', async () => {
  const user = userEvent.setup()
  const { onConfirm } = renderDialog()

  const confirm = await screen.findByTestId('confirm-copy-from-team-button')
  expect(confirm).toBeDisabled()

  await user.click(screen.getByTestId('source-team-picker'))
  await user.click(await screen.findByText('Alpha Team'))

  await waitFor(() => {
    expect(confirm).toBeEnabled()
  })
  await user.click(confirm)
  expect(onConfirm).toHaveBeenCalledWith(alpha)
})

it('tells the page which source was picked so it can build a preview', async () => {
  const user = userEvent.setup()
  const { onSourceChange } = renderDialog()

  await user.click(await screen.findByTestId('source-team-picker'))
  await user.click(await screen.findByText('Alpha Team'))

  expect(onSourceChange).toHaveBeenCalledWith(alpha)
})

it('renders the page-owned preview only once a source is chosen', async () => {
  const user = userEvent.setup()
  renderDialog({ preview: <p>2 types will be added</p> })

  expect(screen.queryByText('2 types will be added')).not.toBeInTheDocument()

  await user.click(await screen.findByTestId('source-team-picker'))
  await user.click(await screen.findByText('Alpha Team'))

  expect(await screen.findByText('2 types will be added')).toBeInTheDocument()
})

it('clears the selection when reopened, so a stale source is never confirmed', async () => {
  const user = userEvent.setup()
  const { rerender } = renderDialog()

  await user.click(await screen.findByTestId('source-team-picker'))
  await user.click(await screen.findByText('Alpha Team'))
  await waitFor(() => {
    expect(screen.getByTestId('confirm-copy-from-team-button')).toBeEnabled()
  })

  // The page closes the dialog itself (as it does after a successful copy),
  // then reopens it.
  const props = {
    onOpenChange: vi.fn(),
    team: destination,
    title: 'Copy artifact types',
    description: 'x',
    submitting: false,
    onConfirm: vi.fn(),
  }
  rerender(<CopyFromTeamDialog open={false} {...props} />)
  rerender(<CopyFromTeamDialog open {...props} />)

  await waitFor(() => {
    expect(screen.getByTestId('confirm-copy-from-team-button')).toBeDisabled()
  })
})

it('clears the selection on close, telling the page to drop its preview', async () => {
  const user = userEvent.setup()
  const onSourceChange = vi.fn()
  const props = {
    onOpenChange: vi.fn(),
    team: destination,
    title: 'Copy artifact types',
    description: 'x',
    submitting: false,
    onConfirm: vi.fn(),
    onSourceChange,
    preview: <p>stale preview</p>,
  }
  const { rerender } = render(<CopyFromTeamDialog open {...props} />)

  await user.click(await screen.findByTestId('source-team-picker'))
  await user.click(await screen.findByText('Alpha Team'))
  expect(onSourceChange).toHaveBeenLastCalledWith(alpha)

  // Closing must reset on the CLOSE edge: resetting on the open edge instead
  // would leave one commit painting the stale team, preview and enabled Copy.
  rerender(<CopyFromTeamDialog open={false} {...props} />)

  await waitFor(() => {
    expect(onSourceChange).toHaveBeenLastCalledWith(null)
  })
})

it('locks the dialog down while the page is submitting', async () => {
  renderDialog({ submitting: true })

  expect(await screen.findByText('Copying…')).toBeInTheDocument()
  expect(screen.getByTestId('confirm-copy-from-team-button')).toBeDisabled()
  expect(screen.getByTestId('source-team-picker')).toBeDisabled()
})
