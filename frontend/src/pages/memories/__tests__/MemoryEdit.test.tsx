import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import type { Memory } from '@/services/memoryService'

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/memoryService', () => ({
  memoryService: { getMemory: vi.fn(), updateMemory: vi.fn() },
}))

vi.mock('@/components/ProjectPicker', () => ({
  ProjectPicker: ({
    value,
    'data-testid': testId,
  }: {
    value: string
    'data-testid'?: string
  }) => <div data-testid={testId}>{value}</div>,
}))

vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
  useAnalytics: () => ({ trackEvent: vi.fn() }),
}))

// Stable across renders on purpose: `loadAll` is a `useCallback` keyed on
// `handleError`, so a fresh mock function per render would re-run the load
// effect forever.
const mockHandleError = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import { memoryService } from '@/services/memoryService'

import { MemoryEdit } from '../MemoryEdit'

const memory: Memory = {
  id: 'mem-1',
  user_id: 'user-1',
  team_id: 'team-1',
  project_id: 'project-1',
  title: 'Deploy checklist',
  text: 'Drain the node first.',
  status: 'active',
  labels: ['ops'],
  // `tags` is lifted into its own chip card; everything else stays in the
  // metadata editor, and the two must never show the same key.
  metadata: { tags: ['runbook'], source: 'wiki', count: 3 },
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  version: 1,
}

function renderEdit() {
  return render(
    <MemoryRouter initialEntries={['/memories/mem-1/edit']}>
      <Routes>
        <Route path="/memories/:id/edit" element={<MemoryEdit />} />
      </Routes>
    </MemoryRouter>
  )
}

async function waitForForm() {
  await waitFor(() => {
    expect(screen.getByTestId('memory-content-textarea')).toBeInTheDocument()
  })
}

describe('MemoryEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn() as () => Promise<void>,
    })
    ;(memoryService.getMemory as Mock).mockResolvedValue(memory)
    ;(memoryService.updateMemory as Mock).mockResolvedValue(memory)
  })

  it('edits the title and the labels #911 and #910 added', async () => {
    renderEdit()
    await waitForForm()

    expect(screen.getByTestId('memory-title-input')).toHaveValue(
      'Deploy checklist'
    )
    expect(screen.getByText('ops')).toBeInTheDocument()
  })

  it('shows tags as chips and never as a metadata row', async () => {
    renderEdit()
    await waitForForm()

    expect(screen.getByText('runbook')).toBeInTheDocument()
    expect(screen.getByTestId('metadata-key-0')).toHaveValue('source')
    expect(screen.queryByDisplayValue('tags')).not.toBeInTheDocument()
  })

  it('recombines chips and metadata rows into one payload', async () => {
    const user = userEvent.setup()
    renderEdit()
    await waitForForm()

    await user.type(screen.getByTestId('memory-tags-input'), 'deploy')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(memoryService.updateMemory).toHaveBeenCalledWith(
        'team-1',
        'mem-1',
        {
          project_id: 'project-1',
          title: 'Deploy checklist',
          text: 'Drain the node first.',
          status: 'active',
          labels: ['ops'],
          // The non-string metadata value survives the round trip.
          metadata: { source: 'wiki', count: 3, tags: ['runbook', 'deploy'] },
        }
      )
    })
  })

  it('clears the title with an explicit null rather than omitting it', async () => {
    const user = userEvent.setup()
    renderEdit()
    await waitForForm()

    await user.clear(screen.getByTestId('memory-title-input'))
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(memoryService.updateMemory).toHaveBeenCalledWith(
        'team-1',
        'mem-1',
        // Omitting the key would leave the old title in place: the API reads a
        // missing `title` as "unchanged" and `null` as "cleared".
        expect.objectContaining({ title: null })
      )
    })
  })

  it('blocks the submit while the body is blank', async () => {
    const user = userEvent.setup()
    renderEdit()
    await waitForForm()

    await user.clear(screen.getByTestId('memory-content-textarea'))
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    expect(await screen.findByText('Memory is required')).toBeInTheDocument()
    expect(memoryService.updateMemory).not.toHaveBeenCalled()
  })
})
