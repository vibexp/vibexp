import { render, screen } from '@testing-library/react'

import type { AgentCard } from '@/services/agentService'

import { AgentPreview } from '../AgentPreview'

const noop = () => undefined

describe('AgentPreview', () => {
  it('renders protocol + transport from the primary interface', () => {
    const data: AgentCard = {
      name: 'Reviewer',
      version: '2.0.0',
      supportedInterfaces: [
        { protocolBinding: 'JSONRPC', protocolVersion: '1.0' },
        { protocolBinding: 'GRPC', protocolVersion: '1.0' },
      ],
    }
    render(
      <AgentPreview loading={false} data={data} error={null} onRetry={noop} />
    )
    expect(screen.getByText('1.0')).toBeInTheDocument()
    expect(screen.getByText('JSONRPC')).toBeInTheDocument()
    expect(screen.queryByText('GRPC')).not.toBeInTheDocument()
  })

  it('falls back to "Not specified" for protocol and transport when empty', () => {
    const data: AgentCard = {
      name: 'Reviewer',
      version: '2.0.0',
      supportedInterfaces: [],
    }
    render(
      <AgentPreview loading={false} data={data} error={null} onRetry={noop} />
    )
    expect(screen.getAllByText('Not specified')).toHaveLength(2)
  })
})
