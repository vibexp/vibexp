import { useCallback, useEffect, useRef, useState } from 'react'

import { type Account, accountService } from '@/services/accountService'

import { accountErrorMessage, isReauthenticationRequired } from './accountRules'

/**
 * The account page's data (#1441): the loaded account and a `change` runner
 * for the calls that alter its sign-in methods.
 *
 * Those calls end the other browser sessions and re-issue this one's cookie,
 * and a request still carrying the old cookie would sign this browser out. So
 * `change` runs one call at a time, lets nothing else start until it has
 * answered, and only then reads the account again.
 */
export function useAccount() {
  const [account, setAccount] = useState<Account | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  // Set when the server refused a change for want of a recent sign-in; the
  // ten minutes can run out while the page is open.
  const [reauthRefused, setReauthRefused] = useState(false)
  const running = useRef(false)

  const load = useCallback(async () => {
    try {
      setLoadError(null)
      setAccount(await accountService.get())
    } catch (err) {
      setLoadError(accountErrorMessage(err, 'Your account could not be loaded'))
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  /**
   * Runs one change. Resolves to true when it succeeded; when it was refused
   * it rethrows so the caller can place the error, having recorded a
   * `reauthentication_required`.
   */
  const change = useCallback(
    async (call: () => Promise<unknown>): Promise<boolean> => {
      if (running.current) return false
      running.current = true
      setBusy(true)
      try {
        await call()
      } catch (err) {
        if (isReauthenticationRequired(err)) setReauthRefused(true)
        throw err
      } finally {
        running.current = false
        setBusy(false)
      }
      await load()
      return true
    },
    [load]
  )

  return {
    account,
    setAccount,
    loadError,
    busy,
    change,
    reload: load,
    /** Whether the changes that need a recent sign-in would be refused. */
    needsSignIn: reauthRefused || account?.recent_sign_in === false,
  }
}

export type AccountState = ReturnType<typeof useAccount>
