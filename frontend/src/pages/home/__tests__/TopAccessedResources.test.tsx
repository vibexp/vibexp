import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import type {
  TeamTopAccessedResourcesResponse,
  TopAccessedResource,
} from '@/types'

const mockGetTeamTopAccessedResources = jest.fn()

jest.mock('@/services/teamService', () => ({
  teamService: {
    getTeamTopAccessedResources: (...args: unknown[]) =>
      mockGetTeamTopAccessedResources(...args),
  },
}))

import { TopAccessedResources } from '../TopAccessedResources'

function item(
  resource_type: string,
  name: string,
  access_count: number,
  id = `${resource_type}-${name}`
): TopAccessedResource {
  return { resource_type, resource_id: id, name, access_count }
}

function buildResponse(
  items: TopAccessedResource[]
): TeamTopAccessedResourcesResponse {
  return {
    status: 'success',
    message: 'ok',
    data: { range: '30d', items },
  }
}

describe('TopAccessedResources', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('renders the four per-type lists with a generous limit', async () => {
    mockGetTeamTopAccessedResources.mockResolvedValue(
      buildResponse([item('artifact', 'How VibeXP works', 57)])
    )

    render(<TopAccessedResources teamId="team-123" range="30d" />)

    await screen.findByText('Top accessed artifacts')
    expect(screen.getByText('Top accessed memories')).toBeInTheDocument()
    expect(screen.getByText('Top accessed blueprints')).toBeInTheDocument()
    expect(screen.getByText('Top accessed prompts')).toBeInTheDocument()

    expect(mockGetTeamTopAccessedResources).toHaveBeenCalledWith(
      'team-123',
      '30d',
      50,
      undefined,
      expect.any(AbortSignal)
    )
  })

  it('buckets items by resource type and caps each list at five', async () => {
    const artifacts = Array.from({ length: 7 }, (_, i) =>
      item('artifact', `Artifact ${String(i)}`, 50 - i, `a-${String(i)}`)
    )
    mockGetTeamTopAccessedResources.mockResolvedValue(
      buildResponse([...artifacts, item('prompt', 'Blog draft outline', 62)])
    )

    render(<TopAccessedResources teamId="team-123" range="30d" />)

    // Top 5 of the 7 artifacts render; the 6th/7th are dropped.
    await screen.findByText('Artifact 0')
    expect(screen.getByText('Artifact 4')).toBeInTheDocument()
    expect(screen.queryByText('Artifact 5')).not.toBeInTheDocument()
    // The prompt lands in its own list.
    expect(screen.getByText('Blog draft outline')).toBeInTheDocument()
  })

  it('shows a per-list empty message for a type with no access', async () => {
    mockGetTeamTopAccessedResources.mockResolvedValue(
      buildResponse([item('artifact', 'Only artifact', 5)])
    )

    render(<TopAccessedResources teamId="team-123" range="30d" />)

    expect(await screen.findByText('No prompt access yet')).toBeInTheDocument()
  })

  it('renders the per-list access-channel filter (All/WEB/CLI/MCP/API)', async () => {
    mockGetTeamTopAccessedResources.mockResolvedValue(
      buildResponse([item('artifact', 'X', 1)])
    )

    render(<TopAccessedResources teamId="team-123" range="30d" />)

    await screen.findByText('Top accessed artifacts')
    // One "All" tab per list (four lists).
    expect(screen.getAllByRole('tab', { name: 'All' })).toHaveLength(4)
  })

  it('re-queries with a source filter when a channel is selected', async () => {
    mockGetTeamTopAccessedResources.mockResolvedValue(
      buildResponse([item('artifact', 'X', 1)])
    )

    render(<TopAccessedResources teamId="team-123" range="30d" />)

    await screen.findByText('Top accessed artifacts')
    // Baseline load is all-channels (source omitted).
    expect(mockGetTeamTopAccessedResources).toHaveBeenLastCalledWith(
      'team-123',
      '30d',
      50,
      undefined,
      expect.any(AbortSignal)
    )

    // Selecting WEB on the first list re-queries with source='web'.
    fireEvent.click(screen.getAllByRole('tab', { name: 'WEB' })[0])
    await waitFor(() => {
      expect(mockGetTeamTopAccessedResources).toHaveBeenLastCalledWith(
        'team-123',
        '30d',
        50,
        'web',
        expect.any(AbortSignal)
      )
    })
  })

  it('shows a per-list load error when a channel fetch fails (not "no access")', async () => {
    // Baseline (all) load succeeds; the channel-filtered fetch then fails.
    mockGetTeamTopAccessedResources
      .mockResolvedValueOnce(buildResponse([item('artifact', 'X', 1)]))
      .mockRejectedValueOnce(new Error('boom'))

    render(<TopAccessedResources teamId="team-123" range="30d" />)

    await screen.findByText('Top accessed artifacts')
    fireEvent.click(screen.getAllByRole('tab', { name: 'CLI' })[0])

    expect(
      await screen.findByText(/couldn't load this channel/i)
    ).toBeInTheDocument()
    // Must not masquerade as an empty channel.
    expect(screen.queryByText('No artifact access yet')).not.toBeInTheDocument()
  })

  it('shows the single empty state when nothing was accessed at all', async () => {
    mockGetTeamTopAccessedResources.mockResolvedValue(buildResponse([]))

    render(<TopAccessedResources teamId="team-123" range="30d" />)

    expect(
      await screen.findByText('No accessed resources yet')
    ).toBeInTheDocument()
    expect(screen.queryByText('Top accessed artifacts')).not.toBeInTheDocument()
  })

  it('shows an error state when the fetch fails', async () => {
    mockGetTeamTopAccessedResources.mockRejectedValue(new Error('boom'))

    render(<TopAccessedResources teamId="team-123" range="30d" />)

    await waitFor(() => {
      expect(
        screen.getByText(/couldn't load top accessed resources/i)
      ).toBeInTheDocument()
    })
  })
})
