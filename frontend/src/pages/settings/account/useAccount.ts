import { useCallback, useEffect, useRef, useState } from 'react'

import { type Account, accountService } from '@/services/accountService'
import { ApiError } from '@/types/errors'

import {
  accountErrorMessage,
  isReauthenticationRequired,
  SIGN_IN_AGAIN_HREF,
} from './accountRules'

/** A refusal that changed nothing: the request, the session age, the throttle. */
const NOTHING_CHANGED = new Set([400, 403, 429])

function statusOf(err: unknown): number | null {
  return err instanceof ApiError ? err.status : null
}

// One stable function: it is a dependency of the loader.
function leaveForSignIn(url: string): void {
  globalThis.location.assign(url)
}

/**
 * The account page's data (#1441): the loaded account and ONE gate that every
 * request of the page goes through.
 *
 * Changing a password or a sign-in method ends the other browser sessions and
 * re-issues this one's cookie; a request still carrying the old cookie that
 * reaches the server afterwards gets a 401 that signs this browser out. So the
 * page never has two requests in flight: `exclusive` runs one piece of work at
 * a time and `busy` stays true until it has fully settled, including the
 * re-read that follows a change.
 *
 * A 401 here is a real sign-out, not something to retry: the page leaves for
 * the sign-in page, which brings the person back.
 *
 * `leave` is injectable so a test does not navigate.
 */
export function useAccount(leave: (url: string) => void = leaveForSignIn) {
  const [account, setAccount] = useState<Account | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  // Set when the server refused a change for want of a recent sign-in; the
  // ten minutes can run out while the page is open.
  const [reauthRefused, setReauthRefused] = useState(false)
  const running = useRef(false)
  // Held in a ref so the loader does not depend on the caller passing a
  // stable function.
  const leaveRef = useRef(leave)
  useEffect(() => {
    leaveRef.current = leave
  }, [leave])

  /** Reads the account. On failure the stale one is not kept on screen. */
  const read = useCallback(async () => {
    try {
      setLoadError(null)
      setAccount(await accountService.get())
    } catch (err) {
      if (statusOf(err) === 401) {
        leaveRef.current(SIGN_IN_AGAIN_HREF)
        return
      }
      setAccount(null)
      setLoadError(accountErrorMessage(err, 'Your account could not be loaded'))
    }
  }, [])

  /**
   * Runs `work` with nothing else of this page in flight. Resolves to false,
   * having done nothing, when something is already running. A 401 from any of
   * the page's requests leaves for the sign-in page before it is rethrown.
   */
  const exclusive = useCallback(
    async (work: () => Promise<void>): Promise<boolean> => {
      if (running.current) return false
      running.current = true
      setBusy(true)
      try {
        await work()
        return true
      } catch (err) {
        if (statusOf(err) === 401) leaveRef.current(SIGN_IN_AGAIN_HREF)
        throw err
      } finally {
        running.current = false
        setBusy(false)
      }
    },
    []
  )

  useEffect(() => {
    void exclusive(read)
  }, [exclusive, read])

  /**
   * Runs one change to the sign-in methods, then reads the account again.
   * Resolves to true when it succeeded and false when it was not sent
   * (something else was running). A refusal is rethrown for the caller to
   * place, after a `reauthentication_required` was recorded and, unless the
   * refusal plainly changed nothing, the account was read again: a 404, a 409
   * or a 5xx can mean the page is stale or the change was partly applied.
   */
  const change = useCallback(
    (call: () => Promise<unknown>): Promise<boolean> =>
      exclusive(async () => {
        try {
          await call()
        } catch (err) {
          const status = statusOf(err)
          // A 401 is a sign-out (`exclusive` leaves); nothing to read.
          if (status === 401) throw err
          if (isReauthenticationRequired(err)) setReauthRefused(true)
          if (status === null || !NOTHING_CHANGED.has(status)) await read()
          throw err
        }
        await read()
      }),
    [exclusive, read]
  )

  return {
    account,
    setAccount,
    loadError,
    busy,
    exclusive,
    change,
    /** Reads the account again, through the gate. */
    reload: useCallback(() => exclusive(read), [exclusive, read]),
    /** The server refused a change because the sign-in is not recent. */
    reauthRefused,
    /** Whether the changes that need a recent sign-in would be refused. */
    needsSignIn: reauthRefused || account?.recent_sign_in === false,
  }
}

export type AccountState = ReturnType<typeof useAccount>
