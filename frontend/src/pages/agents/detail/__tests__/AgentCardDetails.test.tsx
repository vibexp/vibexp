import { render, screen } from '@testing-library/react'

import type { Agent, AgentCard } from '@/services/agentService'

import { AgentCardDetails } from '../AgentCardDetails'

function makeAgent(agent_card: AgentCard | null): Agent {
  return {
    id: 'agent_1',
    user_id: 'user_1',
    team_id: '550e8400-e29b-41d4-a716-446655440000',
    name: 'Code Reviewer',
    description: 'Reviews code',
    status: 'active',
    config: null,
    total_runs: 0,
    success_rate: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    version: 1,
    agent_card,
  }
}

describe('AgentCardDetails', () => {
  it('returns null when the agent has no card', () => {
    const { container } = render(<AgentCardDetails agent={makeAgent(null)} />)
    expect(container.firstChild).toBeNull()
  })

  it("renders the primary interface's protocolBinding as the transport", () => {
    render(
      <AgentCardDetails
        agent={makeAgent({
          supportedInterfaces: [
            { url: 'https://a.example/rpc', protocolBinding: 'JSONRPC' },
            { url: 'https://a.example/grpc', protocolBinding: 'GRPC' },
          ],
        })}
      />
    )
    expect(screen.getByText('Transport')).toBeInTheDocument()
    // The first interface is the primary one shown.
    expect(screen.getByText('JSONRPC')).toBeInTheDocument()
    expect(screen.queryByText('GRPC')).not.toBeInTheDocument()
  })

  it('falls back to "Not specified" when supportedInterfaces is empty', () => {
    render(<AgentCardDetails agent={makeAgent({ supportedInterfaces: [] })} />)
    expect(screen.getByText('Not specified')).toBeInTheDocument()
  })

  it('falls back to "Not specified" when supportedInterfaces is null', () => {
    render(
      <AgentCardDetails agent={makeAgent({ supportedInterfaces: null })} />
    )
    expect(screen.getByText('Not specified')).toBeInTheDocument()
  })
})
