import { useEffect, useState } from 'react'

import { authService } from '@/services/authService'

/**
 * Whether password sign-in is switched on for the instance, as the public
 * sign-in methods report it; null while unknown (or unreadable), so a caller
 * does not disable anything on a guess. Setup and reset links need it (#1443).
 */
export function usePasswordLoginEnabled(): boolean | null {
  const [enabled, setEnabled] = useState<boolean | null>(null)
  useEffect(() => {
    let active = true
    authService
      .getSignInMethods()
      .then(methods => {
        if (active) setEnabled(methods.password.enabled)
      })
      .catch(() => {
        // Unknown: leave the actions to the server's answer.
      })
    return () => {
      active = false
    }
  }, [])
  return enabled
}
