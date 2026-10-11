import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ApiError } from '@/types/errors'
import type { ResourceLock, ResourceLockEvent } from '@/types/resourceLock'

const mockTeam = vi.hoisted(() => ({
  currentTeam: null as {
    id: string
    role?: string
    permissions: string[]
  } | null,
}))
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockTeam,
}))
const mockAuth = vi.hoisted((): { user: { id: string } | null } => ({
  user: { id: 'user-1' },
}))
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockAuth,
}))
const mockShowSuccess = vi.hoisted(() => vi.fn())
vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: mockShowSuccess }),
}))
const mockHandleError = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import { type ResourceLockOps, useResourceLock } from '../useResourceLock'

interface Doc {
  id: string
  lock: ResourceLock | null
}

const lockedBy = (userId: string, name = 'Ada Lovelace'): ResourceLock => ({
  locked_by: { user_id: userId, display_name: name },
  comment: 'Used by the nightly release agent',
  locked_at: '2026-10-11T10:15:30Z',
})

const apiError = (status: number, code: string, detail: string) =>
  new ApiError({
    type: 'about:blank',
    title: 'Error',
    status,
    code,
    detail,
    request_id: 'req-1',
    timestamp: '2026-10-11T10:15:30Z',
  })

function makeOps() {
  return {
    lock: vi.fn<ResourceLockOps<Doc>['lock']>(),
    unlock: vi.fn<ResourceLockOps<Doc>['unlock']>(),
    reload: vi.fn<ResourceLockOps<Doc>['reload']>(),
    loadHistory: vi
      .fn<ResourceLockOps<Doc>['loadHistory']>()
      .mockResolvedValue([]),
  }
}

/** Renders what a detail page renders from the hook. */
function Harness({
  resource,
  ops,
  onChanged,
}: Readonly<{
  resource: Doc | null
  ops: ResourceLockOps<Doc> | null
  onChanged: (doc: Doc) => void
}>) {
  const ui = useResourceLock<Doc>({
    kind: 'artifact',
    resource,
    ops,
    onChanged,
  })
  return (
    <div>
      <div data-testid="actions">
        {ui.actions.map(action => (
          <button
            key={action.id}
            type="button"
            data-testid={action.testId}
            disabled={action.disabled}
            onClick={action.onClick}
          >
            {action.label}
          </button>
        ))}
      </div>
      {ui.historySection.map(section => (
        <section key={section.id} data-section={section.id}>
          <h2>{section.label}</h2>
          {section.content}
        </section>
      ))}
      {ui.dialog}
    </div>
  )
}

function setTeam(permissions: string[], role = 'member') {
  mockTeam.currentTeam = { id: 'team-1', role, permissions }
}

const actionLabels = () =>
  within(screen.getByTestId('actions'))
    .queryAllByRole('button')
    .map(button => button.textContent)

beforeEach(() => {
  vi.clearAllMocks()
  mockAuth.user = { id: 'user-1' }
  setTeam(['resource.create', 'resource.update.any', 'resource.delete.own'])
})

describe('useResourceLock — which action is offered', () => {
  it('offers Lock to any member on an unlocked resource', () => {
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={makeOps()}
        onChanged={vi.fn()}
      />
    )
    expect(actionLabels()).toEqual(['Lock'])
    expect(screen.getByTestId('lock-artifact-button')).toBeEnabled()
  })

  it('offers Unlock to the member who holds the lock', () => {
    render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-1') }}
        ops={makeOps()}
        onChanged={vi.fn()}
      />
    )
    expect(actionLabels()).toEqual(['Unlock'])
    expect(screen.getByTestId('unlock-artifact-button')).toBeInTheDocument()
  })

  it('offers neither Lock nor Unlock to another member on a locked resource', () => {
    render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-2') }}
        ops={makeOps()}
        onChanged={vi.fn()}
      />
    )
    expect(actionLabels()).toEqual([])
  })

  it("offers Unlock to a holder of resource.unlock.any on someone else's lock", () => {
    setTeam(['resource.unlock.any'])
    render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-2') }}
        ops={makeOps()}
        onChanged={vi.fn()}
      />
    )
    expect(actionLabels()).toEqual(['Unlock'])
  })

  it('gates on the permissions array, never on role', () => {
    // An "owner" whose server-published permissions lack resource.unlock.any
    // is not offered Unlock; a "member" who holds it is.
    setTeam([], 'owner')
    const { unmount } = render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-2') }}
        ops={makeOps()}
        onChanged={vi.fn()}
      />
    )
    expect(actionLabels()).toEqual([])
    unmount()

    setTeam(['resource.unlock.any'], 'member')
    render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-2') }}
        ops={makeOps()}
        onChanged={vi.fn()}
      />
    )
    expect(actionLabels()).toEqual(['Unlock'])
  })

  it('offers nothing while the resource or its requests are missing', () => {
    const { rerender } = render(
      <Harness resource={null} ops={null} onChanged={vi.fn()} />
    )
    expect(actionLabels()).toEqual([])
    rerender(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={null}
        onChanged={vi.fn()}
      />
    )
    expect(actionLabels()).toEqual([])
  })
})

describe('useResourceLock — locking', () => {
  it('locks with the trimmed comment and hands the page the updated resource', async () => {
    const user = userEvent.setup()
    const updated: Doc = { id: 'a-1', lock: lockedBy('user-1') }
    const ops = makeOps()
    ops.lock.mockResolvedValue(updated)
    const onChanged = vi.fn()
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={onChanged}
      />
    )

    await user.click(screen.getByTestId('lock-artifact-button'))
    await user.type(screen.getByLabelText(/reason/i), '  in use  ')
    await user.click(screen.getByTestId('confirm-lock-button'))

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledWith(updated)
    })
    expect(ops.lock).toHaveBeenCalledTimes(1)
    expect(ops.lock).toHaveBeenCalledWith('in use')
    expect(mockShowSuccess).toHaveBeenCalledWith('Artifact locked', 'Success')
    expect(ops.reload).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('does not send a lock request for an empty comment', async () => {
    const user = userEvent.setup()
    const ops = makeOps()
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('lock-artifact-button'))
    await user.type(screen.getByLabelText(/reason/i), '   ')

    expect(screen.getByTestId('confirm-lock-button')).toBeDisabled()
    expect(ops.lock).not.toHaveBeenCalled()
  })

  it('refetches and shows the server message when someone else locked first (409 RESOURCE_LOCKED)', async () => {
    const user = userEvent.setup()
    const conflict = apiError(
      409,
      'RESOURCE_LOCKED',
      'The artifact is already locked by Grace Hopper.'
    )
    const reloaded: Doc = {
      id: 'a-1',
      lock: lockedBy('user-2', 'Grace Hopper'),
    }
    const ops = makeOps()
    ops.lock.mockRejectedValue(conflict)
    ops.reload.mockResolvedValue(reloaded)
    const onChanged = vi.fn()
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={onChanged}
      />
    )

    await user.click(screen.getByTestId('lock-artifact-button'))
    await user.type(screen.getByLabelText(/reason/i), 'in use')
    await user.click(screen.getByTestId('confirm-lock-button'))

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledWith(reloaded)
    })
    // The error handler renders an ApiError's own detail: the server's message.
    expect(mockHandleError).toHaveBeenCalledWith(
      conflict,
      'Failed to lock artifact'
    )
    expect(ops.reload).toHaveBeenCalledTimes(1)
    expect(mockShowSuccess).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('keeps the dialog open and does not refetch on any other failure', async () => {
    const user = userEvent.setup()
    const failure = apiError(500, 'INTERNAL_ERROR', 'Something went wrong.')
    const ops = makeOps()
    ops.lock.mockRejectedValue(failure)
    const onChanged = vi.fn()
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={onChanged}
      />
    )

    await user.click(screen.getByTestId('lock-artifact-button'))
    await user.type(screen.getByLabelText(/reason/i), 'in use')
    await user.click(screen.getByTestId('confirm-lock-button'))

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        failure,
        'Failed to lock artifact'
      )
    })
    expect(ops.reload).not.toHaveBeenCalled()
    expect(onChanged).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByLabelText(/reason/i)).toHaveValue('in use')
  })

  it('does not treat a 409 with another code as a stale lock', async () => {
    const user = userEvent.setup()
    const ops = makeOps()
    ops.lock.mockRejectedValue(apiError(409, 'CONFLICT', 'Version conflict.'))
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('lock-artifact-button'))
    await user.type(screen.getByLabelText(/reason/i), 'in use')
    await user.click(screen.getByTestId('confirm-lock-button'))

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledTimes(1)
    })
    expect(ops.reload).not.toHaveBeenCalled()
  })
})

describe('useResourceLock — unlocking', () => {
  it('unlocks and hands the page the updated resource', async () => {
    const user = userEvent.setup()
    const updated: Doc = { id: 'a-1', lock: null }
    const ops = makeOps()
    ops.unlock.mockResolvedValue(updated)
    const onChanged = vi.fn()
    render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-1') }}
        ops={ops}
        onChanged={onChanged}
      />
    )

    await user.click(screen.getByTestId('unlock-artifact-button'))

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledWith(updated)
    })
    expect(ops.unlock).toHaveBeenCalledTimes(1)
    expect(mockShowSuccess).toHaveBeenCalledWith('Artifact unlocked', 'Success')
  })

  it('refetches and shows the server message when someone else unlocked first (409 RESOURCE_NOT_LOCKED)', async () => {
    const user = userEvent.setup()
    const conflict = apiError(
      409,
      'RESOURCE_NOT_LOCKED',
      'The artifact is not locked.'
    )
    const reloaded: Doc = { id: 'a-1', lock: null }
    const ops = makeOps()
    ops.unlock.mockRejectedValue(conflict)
    ops.reload.mockResolvedValue(reloaded)
    const onChanged = vi.fn()
    render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-1') }}
        ops={ops}
        onChanged={onChanged}
      />
    )

    await user.click(screen.getByTestId('unlock-artifact-button'))

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledWith(reloaded)
    })
    expect(mockHandleError).toHaveBeenCalledWith(
      conflict,
      'Failed to unlock artifact'
    )
    expect(mockShowSuccess).not.toHaveBeenCalled()
  })

  it('reports a failed refetch after a stale lock instead of swallowing it', async () => {
    const user = userEvent.setup()
    const reloadFailure = new Error('network down')
    const ops = makeOps()
    ops.unlock.mockRejectedValue(
      apiError(409, 'RESOURCE_NOT_LOCKED', 'The artifact is not locked.')
    )
    ops.reload.mockRejectedValue(reloadFailure)
    const onChanged = vi.fn()
    render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-1') }}
        ops={ops}
        onChanged={onChanged}
      />
    )

    await user.click(screen.getByTestId('unlock-artifact-button'))

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        reloadFailure,
        'Failed to unlock artifact'
      )
    })
    expect(onChanged).not.toHaveBeenCalled()
  })

  it('drops a response that lands after the reader moved to another resource', async () => {
    const user = userEvent.setup()
    let resolveUnlock: (doc: Doc) => void = () => undefined
    const ops = makeOps()
    ops.unlock.mockReturnValue(
      new Promise<Doc>(resolve => {
        resolveUnlock = resolve
      })
    )
    const onChanged = vi.fn()
    const { rerender } = render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-1') }}
        ops={ops}
        onChanged={onChanged}
      />
    )

    await user.click(screen.getByTestId('unlock-artifact-button'))
    rerender(
      <Harness
        resource={{ id: 'a-2', lock: null }}
        ops={ops}
        onChanged={onChanged}
      />
    )
    resolveUnlock({ id: 'a-1', lock: null })

    await waitFor(() => {
      expect(screen.getByTestId('lock-artifact-button')).toBeEnabled()
    })
    expect(onChanged).not.toHaveBeenCalled()
  })
})

describe('useResourceLock — lock history', () => {
  const events: ResourceLockEvent[] = [
    {
      id: 'e-3',
      action: 'lock',
      actor: { user_id: 'user-1', display_name: 'Ada Lovelace' },
      comment: 'Used by the nightly release agent',
      created_at: '2026-10-11T10:15:30Z',
    },
    {
      id: 'e-2',
      action: 'unlock',
      actor: null,
      comment: null,
      created_at: '2026-10-10T09:00:00Z',
    },
    {
      id: 'e-1',
      action: 'lock',
      actor: { user_id: 'user-2', display_name: 'Grace Hopper' },
      comment: 'Release freeze',
      created_at: '2026-10-09T08:00:00Z',
    },
  ]

  it('lists lock and unlock events newest first with actor, action, comment and time', async () => {
    const ops = makeOps()
    ops.loadHistory.mockResolvedValue(events)
    render(
      <Harness
        resource={{ id: 'a-1', lock: lockedBy('user-1') }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )

    const items = await screen.findAllByTestId('lock-history-event')
    expect(
      screen.getByRole('heading', { name: 'Lock history', level: 2 })
    ).toBeInTheDocument()
    expect(items.map(item => item.dataset.action)).toEqual([
      'lock',
      'unlock',
      'lock',
    ])
    expect(items[0]).toHaveTextContent(/Ada Lovelace\s*locked/)
    expect(items[0]).toHaveTextContent('Used by the nightly release agent')
    expect(items[2]).toHaveTextContent(/Grace Hopper\s*locked/)
    expect(items[2]).toHaveTextContent('Release freeze')
    // Each event carries its time; the absolute timestamp is the title.
    for (const item of items) {
      expect(item.querySelector('[title]')).not.toBeNull()
    }
  })

  it('renders an event whose actor was deleted as a deleted user', async () => {
    const ops = makeOps()
    ops.loadHistory.mockResolvedValue(events)
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )

    const items = await screen.findAllByTestId('lock-history-event')
    expect(items[1]).toHaveTextContent(/Deleted user\s*unlocked/)
  })

  it('shows no history section for a resource that was never locked', async () => {
    const ops = makeOps()
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )

    await waitFor(() => {
      expect(ops.loadHistory).toHaveBeenCalledTimes(1)
    })
    expect(screen.queryByText('Lock history')).not.toBeInTheDocument()
  })

  it('refetches the history after a lock', async () => {
    const user = userEvent.setup()
    const ops = makeOps()
    ops.loadHistory.mockResolvedValueOnce([]).mockResolvedValueOnce([events[0]])
    ops.lock.mockResolvedValue({ id: 'a-1', lock: lockedBy('user-1') })
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('lock-artifact-button'))
    await user.type(screen.getByLabelText(/reason/i), 'in use')
    await user.click(screen.getByTestId('confirm-lock-button'))

    expect(await screen.findAllByTestId('lock-history-event')).toHaveLength(1)
    expect(ops.loadHistory).toHaveBeenCalledTimes(2)
  })

  it('says so when the history cannot be loaded, without failing the page', async () => {
    const ops = makeOps()
    ops.loadHistory.mockRejectedValue(new Error('boom'))
    render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )

    expect(
      await screen.findByText('Lock history could not be loaded.')
    ).toBeInTheDocument()
    expect(screen.getByTestId('lock-artifact-button')).toBeEnabled()
    expect(mockHandleError).not.toHaveBeenCalled()
  })

  it("drops the previous resource's history when the resource changes", async () => {
    const ops = makeOps()
    ops.loadHistory.mockResolvedValueOnce(events).mockResolvedValueOnce([])
    const { rerender } = render(
      <Harness
        resource={{ id: 'a-1', lock: null }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )
    expect(await screen.findAllByTestId('lock-history-event')).toHaveLength(3)

    rerender(
      <Harness
        resource={{ id: 'a-2', lock: null }}
        ops={ops}
        onChanged={vi.fn()}
      />
    )

    await waitFor(() => {
      expect(screen.queryByTestId('lock-history-event')).not.toBeInTheDocument()
    })
    expect(ops.loadHistory).toHaveBeenCalledTimes(2)
  })
})
