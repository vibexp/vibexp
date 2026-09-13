import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { Agent, AgentCard } from '@/services/agentService'

import { AgentBasicInfo } from '../AgentBasicInfo'

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

describe('AgentBasicInfo', () => {
  it("renders the primary interface's protocolVersion", () => {
    render(
      <AgentBasicInfo
        agent={makeAgent({
          version: '2.0.0',
          supportedInterfaces: [
            { protocolBinding: 'JSONRPC', protocolVersion: '1.0' },
          ],
        })}
      />
    )
    expect(screen.getByText(/Protocol: 1.0/)).toBeInTheDocument()
  })

  it("leaves the card's version to the generated metadata row", () => {
    // One value, one label: `agent_card.version` is a `meta` field on the agent
    // descriptor (#918), so repeating it here would show it twice under two
    // different names.
    render(
      <AgentBasicInfo
        agent={makeAgent({
          version: '2.0.0',
          supportedInterfaces: [
            { protocolBinding: 'JSONRPC', protocolVersion: '1.0' },
          ],
        })}
      />
    )
    expect(screen.queryByText(/2\.0\.0/)).not.toBeInTheDocument()
  })

  it('falls back to "Not specified" when there is no interface', () => {
    render(<AgentBasicInfo agent={makeAgent({ supportedInterfaces: [] })} />)
    expect(screen.getByText(/Protocol: Not specified/)).toBeInTheDocument()
  })

  it('renders the icon tile, not an empty state, when the card is present', () => {
    render(
      <AgentBasicInfo
        agent={makeAgent({
          supportedInterfaces: [
            { protocolBinding: 'JSONRPC', protocolVersion: '1.0' },
          ],
        })}
        onEdit={vi.fn()}
      />
    )
    expect(screen.getByTestId('agent-basic-info')).toBeInTheDocument()
    expect(screen.queryByTestId('empty-state')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Edit agent/ })
    ).not.toBeInTheDocument()
  })

  describe('without an agent card (#951)', () => {
    it('explains that no A2A card is configured when there is no card URL', () => {
      render(
        <AgentBasicInfo
          agent={{ ...makeAgent(null), card_url: null }}
          onEdit={vi.fn()}
        />
      )
      const info = screen.getByTestId('agent-basic-info')
      const empty = within(info).getByTestId('empty-state')
      expect(
        within(empty).getByRole('heading', { name: 'No A2A card configured' })
      ).toBeInTheDocument()
      expect(screen.queryByTestId('agent-card-url')).not.toBeInTheDocument()
      expect(screen.queryByText(/Protocol:/)).not.toBeInTheDocument()
    })

    it('says the card has not been fetched, and shows the URL, when a card URL is set', () => {
      render(
        <AgentBasicInfo
          agent={{
            ...makeAgent(null),
            card_url: 'https://agents.example.com/.well-known/agent.json',
          }}
          onEdit={vi.fn()}
        />
      )
      const empty = screen.getByTestId('empty-state')
      expect(
        within(empty).getByRole('heading', {
          name: 'Agent card not fetched yet',
        })
      ).toBeInTheDocument()
      expect(screen.getByTestId('agent-card-url')).toHaveTextContent(
        'https://agents.example.com/.well-known/agent.json'
      )
      // No promise of a background retry that may never succeed.
      expect(screen.getByTestId('empty-state-message')).not.toHaveTextContent(
        /retry|shortly|automatically/i
      )
    })

    it.each([
      ['no card URL', null],
      ['an unfetched card URL', 'https://agents.example.com/agent.json'],
    ])('offers Edit agent with %s', async (_label, cardUrl) => {
      const onEdit = vi.fn()
      const user = userEvent.setup()
      render(
        <AgentBasicInfo
          agent={{ ...makeAgent(null), card_url: cardUrl }}
          onEdit={onEdit}
        />
      )
      await user.click(screen.getByRole('button', { name: /Edit agent/ }))
      expect(onEdit).toHaveBeenCalledTimes(1)
    })

    it('renders no action when the page supplies no edit affordance', () => {
      render(<AgentBasicInfo agent={{ ...makeAgent(null), card_url: null }} />)
      expect(screen.getByTestId('empty-state')).toBeInTheDocument()
      expect(screen.queryByRole('button')).not.toBeInTheDocument()
    })
  })

  it('leaves the name, description and status to the reading shell', () => {
    // #918 moved all three into the ResourceReadingPage header; repeating them
    // here would put the same status badge on screen twice.
    render(<AgentBasicInfo agent={makeAgent(null)} />)
    expect(screen.queryByText('Code Reviewer')).not.toBeInTheDocument()
    expect(screen.queryByText('Reviews code')).not.toBeInTheDocument()
    expect(screen.queryByText('Active')).not.toBeInTheDocument()
  })
})
