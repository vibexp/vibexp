import { render, screen } from '@testing-library/react'

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
    expect(screen.getByText('Protocol: 1.0')).toBeInTheDocument()
    expect(screen.getByText('Version: 2.0.0')).toBeInTheDocument()
  })

  it('falls back to "Not specified" when there is no interface', () => {
    render(<AgentBasicInfo agent={makeAgent({ supportedInterfaces: [] })} />)
    expect(screen.getByText('Protocol: Not specified')).toBeInTheDocument()
  })

  it.each([
    ['active', 'bg-success'],
    ['paused', 'bg-muted'],
    ['error', 'bg-destructive'],
  ] as const)(
    'badges %s from the shared agent status field',
    (status, toneClass) => {
      // The detail page and the agents list read one `FieldSpec` (#907). A raw
      // <Badge> here would re-fork them — same agent, different colour on
      // /agents and /agents/:id — with agentStatus.test.ts still green, so the
      // guard has to be on this component, not on the spec. Every status is
      // exercised: a hardcoded map that happens to agree on `active` still
      // diverges on the other two.
      render(<AgentBasicInfo agent={{ ...makeAgent(null), status }} />)

      expect(screen.getByText(/^(Active|Paused|Error)$/)).toHaveClass(toneClass)
    }
  )
})
