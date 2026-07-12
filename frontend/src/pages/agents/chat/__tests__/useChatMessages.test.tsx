import { act, renderHook, waitFor } from '@testing-library/react'

import type { AgentExecutionEvent } from '@/hooks/useEventPolling'
import type { Agent, AgentExecution } from '@/services/agentService'

import { type Message, PLACEHOLDER_TEXT, STREAMING } from '../types'
import {
  extractResponseText,
  updateStreamingMessages,
  useChatMessages,
} from '../useChatMessages'

// Capture the props useChatMessages passes to useEventPolling so tests can drive
// onEvent / onComplete without a real polling loop.
const mockUseEventPolling = jest.fn()
jest.mock('@/hooks/useEventPolling', () => ({
  useEventPolling: (props: unknown) => {
    mockUseEventPolling(props)
    return { currentState: null }
  },
}))

jest.mock('@/services/agentService', () => ({
  agentService: {
    executeAgent: jest.fn(),
    getExecutionStatus: jest.fn(),
    getConversationExecutions: jest.fn(),
    cancelExecution: jest.fn(),
  },
}))

jest.mock('@/lib/toast', () => ({
  toast: { error: jest.fn(), success: jest.fn() },
}))

import { agentService } from '@/services/agentService'

interface CapturedPollingProps {
  teamId?: string
  executionId: string | null
  enabled: boolean
  onEvent: (event: AgentExecutionEvent) => void
  onComplete: () => void
}

function latestPolling(): CapturedPollingProps {
  const calls = mockUseEventPolling.mock.calls
  return calls[calls.length - 1][0] as CapturedPollingProps
}

function artifactEvent(
  executionId: string,
  sequence: number,
  text: string,
  append: boolean
): AgentExecutionEvent {
  return {
    execution_id: executionId,
    sequence_number: sequence,
    event_type: 'artifact-update',
    event_data: {
      artifact: { artifactId: 'a1', parts: [{ text }] },
      append,
    },
  } as unknown as AgentExecutionEvent
}

describe('updateStreamingMessages', () => {
  const streaming = (text: string, artifactId?: string): Message => ({
    role: 'agent',
    text,
    timestamp: STREAMING,
    artifactId,
  })

  it('replaces the placeholder with the first chunk', () => {
    const prev: Message[] = [
      { role: 'user', text: 'hi', timestamp: 't' },
      { role: 'agent', text: PLACEHOLDER_TEXT, timestamp: STREAMING },
    ]
    const next = updateStreamingMessages(prev, 'chunk1', false, 'a1')
    expect(next[1]).toMatchObject({
      role: 'agent',
      text: 'chunk1',
      timestamp: STREAMING,
      artifactId: 'a1',
    })
  })

  it('appends to an in-progress streaming message with the same artifactId', () => {
    const prev: Message[] = [
      { role: 'user', text: 'hi', timestamp: 't' },
      streaming('chunk1', 'a1'),
    ]
    const next = updateStreamingMessages(prev, 'chunk2', true, 'a1')
    expect(next[1].text).toBe('chunk1chunk2')
  })

  it('ignores a non-append chunk while a streaming message is in progress', () => {
    const prev: Message[] = [
      { role: 'user', text: 'hi', timestamp: 't' },
      streaming('chunk1', 'a1'),
    ]
    expect(updateStreamingMessages(prev, 'other', false, 'a2')).toBe(prev)
  })

  it('finalizes the previous message and starts a new streaming message otherwise', () => {
    const prev: Message[] = [
      { role: 'agent', text: 'done', timestamp: 'fixed' },
    ]
    const next = updateStreamingMessages(prev, 'newchunk', false, 'a1')
    expect(next).toHaveLength(2)
    expect(next[0].timestamp).not.toBe(STREAMING)
    expect(next[1]).toMatchObject({
      text: 'newchunk',
      timestamp: STREAMING,
      artifactId: 'a1',
    })
  })
})

describe('extractResponseText', () => {
  it('returns an Error: message for a failed execution', () => {
    expect(
      extractResponseText({ status: 'error', error: 'nope' } as AgentExecution)
    ).toBe('Error: nope')
  })

  it('joins text parts, handling v1.0 no-kind parts via isTextPart', () => {
    expect(
      extractResponseText({
        status: 'success',
        artifacts: [
          { artifactId: 'a', parts: [{ text: 'hello' }, { text: 'world' }] },
        ],
      } as unknown as AgentExecution)
    ).toBe('hello\nworld')
  })

  it('returns Cancelled for a cancelled execution with no artifacts', () => {
    expect(extractResponseText({ status: 'cancelled' } as AgentExecution)).toBe(
      'Cancelled'
    )
  })

  it('falls back to "No response received"', () => {
    expect(extractResponseText({ status: 'success' } as AgentExecution)).toBe(
      'No response received'
    )
  })
})

describe('useChatMessages', () => {
  const agent = {
    id: 'agent-1',
    agent_card: { defaultInputModes: ['text'] },
  } as unknown as Agent

  const baseArgs = {
    teamId: 'team-1',
    agent,
    conversationId: null,
    onConversationCaptured: jest.fn(),
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('sendMessage completes synchronously for a terminal execution (no polling)', async () => {
    jest.mocked(agentService.executeAgent).mockResolvedValue({
      id: 'e1',
      status: 'completed',
      artifacts: [{ artifactId: 'a', parts: [{ text: 'done' }] }],
    } as unknown as AgentExecution)

    const { result } = renderHook(() => useChatMessages(baseArgs))
    await act(async () => {
      await result.current.sendMessage('hi')
    })

    expect(result.current.messages.map(m => m.role)).toEqual(['user', 'agent'])
    expect(result.current.messages[1].text).toBe('done')
    expect(result.current.currentExecutionId).toBeNull()
    expect(result.current.isExecuting).toBe(false)
    expect(result.current.executionMetadata?.taskId).toBe('e1')
  })

  it('sendMessage starts polling for a pending execution', async () => {
    jest.mocked(agentService.executeAgent).mockResolvedValue({
      id: 'e2',
      status: 'pending',
    } as unknown as AgentExecution)

    const { result } = renderHook(() => useChatMessages(baseArgs))
    await act(async () => {
      await result.current.sendMessage('hi')
    })

    expect(result.current.currentExecutionId).toBe('e2')
    expect(result.current.isExecuting).toBe(true)
    expect(latestPolling().enabled).toBe(true)
    expect(latestPolling().executionId).toBe('e2')
  })

  it('assembles streaming artifact-update chunks', async () => {
    jest.mocked(agentService.executeAgent).mockResolvedValue({
      id: 'e3',
      status: 'pending',
    } as unknown as AgentExecution)

    const { result } = renderHook(() => useChatMessages(baseArgs))
    await act(async () => {
      await result.current.sendMessage('hi')
    })

    act(() => {
      latestPolling().onEvent(artifactEvent('e3', 0, 'Hello ', false))
    })
    act(() => {
      latestPolling().onEvent(artifactEvent('e3', 1, 'world', true))
    })

    expect(result.current.messages.at(-1)?.text).toBe('Hello world')
  })

  it('dedups a streaming event delivered twice (same execution + sequence)', async () => {
    jest.mocked(agentService.executeAgent).mockResolvedValue({
      id: 'e6',
      status: 'pending',
    } as unknown as AgentExecution)

    const { result } = renderHook(() => useChatMessages(baseArgs))
    await act(async () => {
      await result.current.sendMessage('hi')
    })

    act(() => {
      latestPolling().onEvent(artifactEvent('e6', 0, 'Hello ', false))
    })
    act(() => {
      latestPolling().onEvent(artifactEvent('e6', 1, 'world', true))
    })
    // Same seq=1 event again — must be ignored, not appended twice.
    act(() => {
      latestPolling().onEvent(artifactEvent('e6', 1, 'world', true))
    })

    expect(result.current.messages.at(-1)?.text).toBe('Hello world')
  })

  it('captures the conversation id on completion when none was set', async () => {
    jest.mocked(agentService.executeAgent).mockResolvedValue({
      id: 'e4',
      status: 'pending',
    } as unknown as AgentExecution)
    jest.mocked(agentService.getExecutionStatus).mockResolvedValue({
      id: 'e4',
      status: 'success',
      conversation_id: 'conv-9',
      artifacts: [{ artifactId: 'a', parts: [{ text: 'ok' }] }],
    } as unknown as AgentExecution)

    const onConversationCaptured = jest.fn()
    const { result } = renderHook(() =>
      useChatMessages({ ...baseArgs, onConversationCaptured })
    )
    await act(async () => {
      await result.current.sendMessage('hi')
    })

    act(() => {
      latestPolling().onComplete()
    })
    await waitFor(() => {
      expect(onConversationCaptured).toHaveBeenCalledWith('conv-9')
    })
    expect(result.current.messages.at(-1)?.text).toBe('ok')
  })

  it('loadConversation maps executions into messages and pagination flags', async () => {
    jest.mocked(agentService.getConversationExecutions).mockResolvedValue({
      total_count: 1,
      has_more: true,
      executions: [
        {
          id: 'x',
          status: 'success',
          input: { text: 'q' },
          artifacts: [
            { artifactId: 'a', parts: [{ kind: 'text', text: 'reply' }] },
          ],
          started_at: 's',
        },
      ],
    } as unknown as Awaited<
      ReturnType<typeof agentService.getConversationExecutions>
    >)

    const { result } = renderHook(() => useChatMessages(baseArgs))
    await act(async () => {
      await result.current.loadConversation('conv-1')
    })

    expect(result.current.totalMessageCount).toBe(1)
    expect(result.current.hasEarlierMessages).toBe(true)
    expect(result.current.messages.map(m => m.role)).toEqual(['user', 'agent'])
    expect(result.current.messages[1].text).toBe('reply')
  })

  it('cancelExecution finalizes the execution', async () => {
    jest.mocked(agentService.executeAgent).mockResolvedValue({
      id: 'e7',
      status: 'pending',
    } as unknown as AgentExecution)
    jest.mocked(agentService.cancelExecution).mockResolvedValue({
      id: 'e7',
      status: 'cancelled',
    } as unknown as AgentExecution)

    const { result } = renderHook(() => useChatMessages(baseArgs))
    await act(async () => {
      await result.current.sendMessage('hi')
    })
    await act(async () => {
      await result.current.cancelExecution()
    })

    expect(result.current.messages.at(-1)?.text).toBe('Cancelled')
    expect(result.current.currentExecutionId).toBeNull()
  })
})
