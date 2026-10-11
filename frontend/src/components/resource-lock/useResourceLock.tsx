import { Lock, Unlock } from 'lucide-react'
import { type ReactNode, useEffect, useRef, useState } from 'react'

import type {
  ReadingAction,
  ReadingSection,
} from '@/components/patterns/reading-page'
import { useAlerts } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { ApiError } from '@/types/errors'
import type {
  LockableKind,
  ResourceLock,
  ResourceLockEvent,
} from '@/types/resourceLock'

import { lockHistorySection } from './lockHistorySection'
import { LockResourceDialog } from './LockResourceDialog'

/** The part of a prompt, blueprint or artifact payload the lock UI reads. */
export interface LockableResource {
  id: string
  lock: ResourceLock | null
}

/** The lock requests for one resource, already bound to its address. */
export interface ResourceLockOps<T extends LockableResource> {
  /** Locks the resource and resolves with its updated payload. */
  lock: (comment: string) => Promise<T>
  /** Unlocks the resource and resolves with its updated payload. */
  unlock: () => Promise<T>
  /** Refetches the resource, after someone else changed its lock first. */
  reload: () => Promise<T>
  /** Lock and unlock events, newest first. */
  loadHistory: () => Promise<ResourceLockEvent[]>
}

export interface UseResourceLockOptions<T extends LockableResource> {
  kind: LockableKind
  /** The loaded resource, or `null` while it loads or could not be found. */
  resource: T | null
  /**
   * The requests, or `null` while the page lacks the context to make them.
   * Callers build this inline; nothing here keys off its identity.
   */
  ops: ResourceLockOps<T> | null
  /** Receives the resource as the server returned it after a lock change. */
  onChanged: (resource: T) => void
}

export interface ResourceLockUi {
  /** The Lock or Unlock action, or none when the user may do neither. */
  actions: ReadingAction[]
  /** The lock dialog. Render it once, anywhere on the page. */
  dialog: ReactNode
  /** The "Lock history" section, for the page's `extraSections`. */
  historySection: ReadingSection[]
}

/** The 409 codes that mean someone else locked or unlocked first. */
const STALE_LOCK_CODES = new Set(['RESOURCE_LOCKED', 'RESOURCE_NOT_LOCKED'])

const isStaleLockError = (error: unknown): boolean =>
  error instanceof ApiError &&
  error.status === 409 &&
  STALE_LOCK_CODES.has(error.code)

/**
 * Lock and unlock for a resource detail page (epic #1285): the Lock / Unlock
 * action, the lock dialog and the lock history section, shared by the prompt,
 * blueprint and artifact pages so each adds a few lines instead of a copy.
 *
 * Any member may lock an unlocked resource. Unlock is offered to the locker
 * and to holders of `resource.unlock.any` — read from the team's `permissions`
 * array through `usePermissions`, never from `role`. Another member sees
 * neither action on a locked resource. The server authorizes every request
 * regardless; this only decides what is offered.
 */
export function useResourceLock<T extends LockableResource>({
  kind,
  resource,
  ops,
  onChanged,
}: UseResourceLockOptions<T>): ResourceLockUi {
  const { canUnlockResource } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [events, setEvents] = useState<ResourceLockEvent[]>([])
  const [historyError, setHistoryError] = useState<string | null>(null)
  // Bumped after every lock change so the history refetches.
  const [revision, setRevision] = useState(0)

  // The callers' closures are rebuilt every render. Effects and in-flight
  // requests read the latest ones here instead of depending on identities.
  const latest = useRef({ resource, ops, onChanged })
  useEffect(() => {
    latest.current = { resource, ops, onChanged }
  })

  const resourceId = resource?.id
  const ready = ops !== null

  useEffect(() => {
    // Whatever is held belongs to the resource that has just been replaced.
    setEvents(previous => (previous.length === 0 ? previous : []))
    setHistoryError(null)
    setDialogOpen(false)
  }, [resourceId])

  useEffect(() => {
    const loadHistory = latest.current.ops?.loadHistory
    if (resourceId === undefined || !loadHistory) return
    // Guard against a slower response for a previously viewed resource.
    let active = true
    const load = async () => {
      try {
        const loaded = await loadHistory()
        if (!active) return
        setEvents(loaded)
        setHistoryError(null)
      } catch {
        // Best-effort: a failed history load must never break the page.
        if (active) setHistoryError('Lock history could not be loaded.')
      }
    }
    void load()
    return () => {
      active = false
    }
  }, [resourceId, ready, revision])

  /**
   * Runs one lock change. Resolves `true` when it was applied. A 409 that
   * means the lock changed under the reader refetches the resource and shows
   * the server's message, rather than trusting what this page last saw.
   */
  const run = async (
    change: (ops: ResourceLockOps<T>) => Promise<T>,
    success: string,
    failure: string
  ): Promise<boolean> => {
    const started = latest.current
    if (!started.ops || !started.resource || busy) return false
    const startedId = started.resource.id
    // A response for a resource the reader has since left is dropped.
    const apply = (updated: T) => {
      if (latest.current.resource?.id !== startedId) return
      latest.current.onChanged(updated)
      setRevision(n => n + 1)
    }
    setBusy(true)
    try {
      apply(await change(started.ops))
      showSuccess(success, 'Success')
      return true
    } catch (error) {
      handleError(error, failure)
      if (!isStaleLockError(error)) return false
      try {
        apply(await started.ops.reload())
      } catch (reloadError) {
        handleError(reloadError, failure)
      }
      // The dialog has nothing left to submit: the lock state it was opened
      // for is gone.
      setDialogOpen(false)
      return false
    } finally {
      setBusy(false)
    }
  }

  // "Prompt locked", "Failed to lock prompt".
  const label = kind.charAt(0).toUpperCase() + kind.slice(1)

  const submitLock = (comment: string) => {
    void run(
      o => o.lock(comment),
      `${label} locked`,
      `Failed to lock ${kind}`
    ).then(applied => {
      if (applied) setDialogOpen(false)
    })
  }

  const submitUnlock = () => {
    void run(o => o.unlock(), `${label} unlocked`, `Failed to unlock ${kind}`)
  }

  // Built as object literals rather than pushed: the handlers read the latest
  // closures through a ref, which is only safe outside render.
  const lockAction: ReadingAction = {
    id: 'lock',
    label: 'Lock',
    icon: Lock,
    testId: `lock-${kind}-button`,
    disabled: busy,
    onClick: () => {
      setDialogOpen(true)
    },
  }
  const unlockAction: ReadingAction = {
    id: 'unlock',
    label: 'Unlock',
    icon: Unlock,
    testId: `unlock-${kind}-button`,
    disabled: busy,
    onClick: submitUnlock,
  }
  const lock = resource?.lock
  let actions: ReadingAction[] = []
  if (resource && ops) {
    if (!lock) actions = [lockAction]
    else if (canUnlockResource(lock.locked_by.user_id)) actions = [unlockAction]
  }

  return {
    actions,
    dialog: (
      <LockResourceDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        kind={kind}
        submitting={busy}
        onSubmit={submitLock}
      />
    ),
    historySection: lockHistorySection({ events, error: historyError }),
  }
}
