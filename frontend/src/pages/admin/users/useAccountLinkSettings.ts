import { useEffect, useState } from 'react'

import { authService } from '@/services/authService'

export interface AccountLinkSettings {
  /** Whether password sign-in is switched on; setup and reset links need it. */
  passwordLogin: boolean | null
  /**
   * Whether a link can be emailed: an instance email provider is configured
   * (`password.reset` is `email`). False means only "Show the link here" can
   * work (#1494).
   */
  emailLinks: boolean | null
}

const UNKNOWN: AccountLinkSettings = { passwordLogin: null, emailLinks: null }

/**
 * What the instance offers for account links, as the public sign-in methods
 * report it (#1443, #1494). Each value is null while unknown (or unreadable),
 * so a caller does not disable anything on a guess and leaves it to the
 * server's answer.
 */
export function useAccountLinkSettings(): AccountLinkSettings {
  const [settings, setSettings] = useState<AccountLinkSettings>(UNKNOWN)
  useEffect(() => {
    let active = true
    authService
      .getSignInMethods()
      .then(methods => {
        if (active) {
          setSettings({
            passwordLogin: methods.password.enabled,
            emailLinks: methods.password.reset === 'email',
          })
        }
      })
      .catch(() => {
        // Unknown: leave the actions to the server's answer.
      })
    return () => {
      active = false
    }
  }, [])
  return settings
}
