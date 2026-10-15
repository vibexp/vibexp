import { Icon } from '@vibexp/design-system/react'
import { useEffect } from 'react'

import { RelativeTime } from '@/components/RelativeTime'
import { useAuth } from '@/contexts/useAuth'
import { cn } from '@/lib/utils'

import { isLoopRunning, knowledgeLoopOf } from '../knowledgeLoop'

/** How often the status is re-read while the loop is not complete. */
export const LOOP_REFRESH_MS = 10_000

const ROWS = [
  {
    key: 'first_read_at',
    label: 'Your agent read from the shared brain',
    hint: 'Not yet: run /vibexp:prime in a project',
  },
  {
    key: 'first_write_at',
    label: 'Your agent wrote back to the shared brain',
    hint: "Not yet: /vibexp:wrap saves what's worth keeping",
  },
] as const

/**
 * The celebration's live check that the loop is actually running: whether
 * the user's AI tools have read from VibeXP and written back yet.
 *
 * The user is re-read when the step opens, then every few seconds while the
 * step is on screen and the loop is not complete, so a /vibexp:prime run in a
 * terminal shows up here without a reload. It stops once both have happened.
 */
export function KnowledgeLoopStatus() {
  const { user, refreshUser } = useAuth()
  const loop = knowledgeLoopOf(user)
  const running = isLoopRunning(loop)

  useEffect(() => {
    void refreshUser()
  }, [refreshUser])

  useEffect(() => {
    if (running) return
    const timer = setInterval(() => {
      // A background tab has no one to show it to.
      if (!document.hidden) void refreshUser()
    }, LOOP_REFRESH_MS)
    return () => {
      clearInterval(timer)
    }
  }, [running, refreshUser])

  return (
    <div
      role="list"
      aria-label="Knowledge loop"
      className="flex flex-col overflow-hidden rounded-lg border"
    >
      {ROWS.map((row, n) => {
        const at = loop[row.key]
        const done = at !== null
        return (
          <div
            key={row.key}
            role="listitem"
            data-state={done ? 'done' : 'pending'}
            className={cn(
              'flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-sm',
              n > 0 && 'border-t'
            )}
          >
            <span
              aria-hidden="true"
              className={cn(
                'text-primary-foreground grid size-[22px] flex-none place-items-center rounded-full border',
                done ? 'bg-primary border-transparent' : 'bg-transparent'
              )}
            >
              {done && <Icon name="check" size={12} stroke={3} />}
            </span>
            <span
              className={cn(
                'flex-1',
                done ? 'text-foreground' : 'text-muted-foreground'
              )}
            >
              {row.label}
            </span>
            {done ? (
              <RelativeTime
                value={at}
                className="text-muted-foreground text-[13px]"
              />
            ) : (
              <span className="text-muted-foreground text-[13px]">
                {row.hint}
              </span>
            )}
          </div>
        )
      })}
    </div>
  )
}
