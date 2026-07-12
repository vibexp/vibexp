import type { AgentExecution } from '@/services/agentService'

import { convertExecutionToMessages } from '../convertExecution'

function makeExecution(
  overrides: Partial<Omit<AgentExecution, 'artifacts'>> & {
    artifacts?: unknown
  }
): AgentExecution {
  return {
    id: 'exec-1',
    agent_id: 'agent-1',
    user_id: 'user-1',
    status: 'success',
    started_at: '2026-01-01T00:00:00Z',
    version: 1,
    ...overrides,
  } as AgentExecution
}

describe('convertExecutionToMessages', () => {
  it('maps an input + a text artifact to a user then an agent message', () => {
    const messages = convertExecutionToMessages(
      makeExecution({
        input: { text: 'hello there' },
        artifacts: [
          { artifactId: 'a1', parts: [{ kind: 'text', text: 'the answer' }] },
        ],
        ended_at: '2026-01-01T00:00:05Z',
      })
    )

    expect(messages).toHaveLength(2)
    expect(messages[0]).toMatchObject({
      role: 'user',
      text: 'hello there',
      timestamp: '2026-01-01T00:00:00Z',
    })
    expect(messages[1]).toMatchObject({
      role: 'agent',
      text: 'the answer',
      isError: false,
      timestamp: '2026-01-01T00:00:05Z',
    })
  })

  it('joins multiple text parts across artifacts with newlines', () => {
    const messages = convertExecutionToMessages(
      makeExecution({
        input: { text: 'q' },
        artifacts: [
          {
            artifactId: 'a1',
            parts: [
              { kind: 'text', text: 'line one' },
              { kind: 'text', text: 'line two' },
            ],
          },
        ],
      })
    )

    expect(messages[1].text).toBe('line one\nline two')
  })

  it('renders an error execution as an error agent message', () => {
    const messages = convertExecutionToMessages(
      makeExecution({ input: { text: 'q' }, status: 'error', error: 'boom' })
    )

    expect(messages[1]).toMatchObject({
      role: 'agent',
      text: 'Error: boom',
      isError: true,
    })
  })

  it('falls back to "No response" for the missing-artifact edge', () => {
    const messages = convertExecutionToMessages(
      makeExecution({ input: { text: 'q' }, artifacts: undefined })
    )

    expect(messages[1]).toMatchObject({ role: 'agent', text: 'No response' })
  })

  it('stringifies a non-text input object', () => {
    const messages = convertExecutionToMessages(
      makeExecution({ input: { foo: 'bar' } })
    )

    expect(messages[0]).toMatchObject({
      role: 'user',
      text: JSON.stringify({ foo: 'bar' }),
    })
  })

  it('omits the user message when input is falsy', () => {
    const messages = convertExecutionToMessages(
      makeExecution({ input: undefined })
    )

    expect(messages).toHaveLength(1)
    expect(messages[0].role).toBe('agent')
  })

  it('falls back to started_at when ended_at is absent', () => {
    const messages = convertExecutionToMessages(
      makeExecution({ input: { text: 'q' }, ended_at: undefined })
    )

    expect(messages[1].timestamp).toBe('2026-01-01T00:00:00Z')
  })

  it('extracts A2A v1.0 text parts that lack a kind (via isTextPart)', () => {
    const messages = convertExecutionToMessages(
      makeExecution({
        input: { text: 'q' },
        artifacts: [{ artifactId: 'a1', parts: [{ text: 'v1.0 reply' }] }],
      })
    )

    expect(messages[1].text).toBe('v1.0 reply')
  })
})
