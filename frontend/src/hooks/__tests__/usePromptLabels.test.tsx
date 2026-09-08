import { act, render, waitFor } from '@testing-library/react'

const mockTeam = vi.hoisted<{ current: { id: string } | null }>(() => ({
  current: { id: 'team-1' },
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: mockTeam.current,
    teams: mockTeam.current ? [mockTeam.current] : [],
    isLoading: false,
  }),
}))

const getPromptLabels = vi.hoisted(() => vi.fn())
vi.mock('@/services/promptService', () => ({
  promptService: { getPromptLabels },
}))

import { usePromptLabels } from '@/hooks/usePromptLabels'

let captured: ReturnType<typeof usePromptLabels>

function Probe() {
  captured = usePromptLabels()
  return null
}

beforeEach(() => {
  vi.clearAllMocks()
  mockTeam.current = { id: 'team-1' }
  getPromptLabels.mockResolvedValue(['api'])
})

describe('usePromptLabels', () => {
  it('fetches nothing until asked', () => {
    render(<Probe />)
    expect(getPromptLabels).not.toHaveBeenCalled()
    expect(captured.labels).toEqual([])
  })

  it('loads the catalog once, however often it is asked', async () => {
    render(<Probe />)
    act(() => {
      captured.load()
      captured.load()
    })
    await waitFor(() => {
      expect(captured.labels).toEqual(['api'])
    })
    act(() => {
      captured.load()
    })
    expect(getPromptLabels.mock.calls).toHaveLength(1)
  })

  it('reports a failure instead of an empty catalog', async () => {
    getPromptLabels.mockRejectedValue(new Error('boom'))
    render(<Probe />)
    act(() => {
      captured.load()
    })
    await waitFor(() => {
      expect(captured.error).toBe('Failed to load labels')
    })
    expect(captured.loading).toBe(false)
  })

  it('does not call the service without a team', () => {
    mockTeam.current = null
    render(<Probe />)
    act(() => {
      captured.load()
    })
    expect(getPromptLabels).not.toHaveBeenCalled()
  })
})
