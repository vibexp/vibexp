import { useState } from 'react'

import { useAuth } from '@/contexts/useAuth'

import { AllowlistSection } from './AllowlistSection'
import { AuthAuditSection } from './AuthAuditSection'
import { InstanceAdminsSection } from './InstanceAdminsSection'
import { ProvidersSection } from './ProvidersSection'
import { useAuthProviders } from './useAuthProviders'

/**
 * Admin → Settings → Authentication (#1239, epic #1230): the sign-in
 * providers, the access allowlist, the instance admins and their audit
 * history, all stored in the database and applied with no restart.
 *
 * The section heading comes from the admin shell (`admin-nav.ts`), so the page
 * renders none. Who may grant or revoke instance admin comes from the server's
 * `is_root_instance_admin` flag on `/auth/me`; the first-run `/setup` page
 * reuses the providers and allowlist sections without this page's shell.
 */
export function AdminAuthSettings() {
  const { user } = useAuth()
  // An allowlist save bumps the shared version the providers are saved
  // against, so it makes the providers re-read theirs.
  const [providersKey, setProvidersKey] = useState(0)
  const [auditKey, setAuditKey] = useState(0)
  const recordChange = () => {
    setAuditKey(key => key + 1)
  }
  const providers = useAuthProviders(providersKey, recordChange)

  return (
    <div className="space-y-6">
      <ProvidersSection state={providers} />
      <AllowlistSection
        onSaved={() => {
          setProvidersKey(key => key + 1)
          recordChange()
        }}
      />
      <InstanceAdminsSection
        canManage={user?.is_root_instance_admin === true}
        onChanged={recordChange}
      />
      <AuthAuditSection refreshKey={auditKey} />
    </div>
  )
}
